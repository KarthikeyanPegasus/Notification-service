package handler

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

var (
	// globalBlockList tracks IPs that hit DDoS thresholds
	globalBlockList = make(map[string]time.Time)
	blockListMu     sync.RWMutex
)

// RequestID injects a unique X-Request-ID into every request.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		c.Set("request_id", reqID)
		c.Header("X-Request-ID", reqID)
		c.Next()
	}
}

// Logger emits a structured log line for every request.
func Logger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		log.Info("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
			zap.String("request_id", c.GetString("request_id")),
		)
	}
}

// Recovery catches panics and returns 500.
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered", zap.Any("error", r))
				c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
					Code:      "INTERNAL_ERROR",
					Message:   "an unexpected error occurred",
					RequestID: c.GetString("request_id"),
				})
			}
		}()
		c.Next()
	}
}

// CORS adds permissive CORS headers for development; tighten for production.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Request-ID,Idempotency-Key")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// JWTAuth validates Bearer tokens and injects claims into the context.
// In debug mode, it allows requests without a token and sets a dummy caller ID.
func JWTAuth(secret string, isDebugMode bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			if isDebugMode {
				c.Set("claims", map[string]interface{}{"sub": "debug-admin", "role": "admin"})
				c.Set("caller_id", "debug-admin")
				c.Next()
				return
			}
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing Authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid Authorization header format")
			c.Abort()
			return
		}

		tokenStr := parts[1]
		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
			c.Abort()
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("claims", claims)
			if sub, ok := claims["sub"].(string); ok {
				c.Set("caller_id", sub)
			}
		}
		c.Next()
	}
}

// ServiceAuth validates internal service tokens (simpler than full JWT).
func ServiceAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Service-Token")
		if token == "" {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing X-Service-Token")
			c.Abort()
			return
		}
		if token != secret {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid service token")
			c.Abort()
			return
		}
		c.Next()
	}
}

// SecurityHeaders adds standard security-focused headers to the response.
func SecurityHeaders(cfg config.HeadersConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.EnableSecureHeaders {
			c.Next()
			return
		}

		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none';")

		c.Next()
	}
}

// RequestSizeLimiter limits the maximum allowed body size.
func RequestSizeLimiter(maxMB int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxMB <= 0 {
			c.Next()
			return
		}
		
		limit := int64(maxMB) * 1024 * 1024
		if c.Request.ContentLength > limit {
			respondError(c, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body exceeds maximum allowed size")
			c.Abort()
			return
		}
		
		// Also wrap the reader to be safe for chunked encoding
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter appies per-IP rate limiting and DDoS prevention.
func RateLimiter(cfg config.SecurityConfig) gin.HandlerFunc {
	limiters := make(map[string]*clientLimiter)
	var mu sync.Mutex

	// Periodic cleanup of stale limiters to prevent memory leaks
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			mu.Lock()
			for ip, cl := range limiters {
				if time.Since(cl.lastSeen) > 10*time.Minute {
					delete(limiters, ip)
				}
			}
			mu.Unlock()

			// Also cleanup expired blocks
			blockListMu.Lock()
			for ip, expiration := range globalBlockList {
				if time.Now().After(expiration) {
					delete(globalBlockList, ip)
				}
			}
			blockListMu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		if !cfg.RateLimit.Enabled {
			c.Next()
			return
		}

		ip := c.ClientIP()

		// Check if IP is currently blocked (DDoS Prevention)
		blockListMu.RLock()
		blockedUntil, isBlocked := globalBlockList[ip]
		blockListMu.RUnlock()

		if isBlocked {
			if time.Now().Before(blockedUntil) {
				c.Header("X-Security-Action", "Blocked")
				respondError(c, http.StatusForbidden, "ACCESS_DENIED", "your IP has been temporarily blocked due to excessive requests")
				c.Abort()
				return
			}
			// Block expired, remove it (cleanup routine will also handle this eventually)
			blockListMu.Lock()
			delete(globalBlockList, ip)
			blockListMu.Unlock()
		}

		mu.Lock()
		cl, ok := limiters[ip]
		if !ok {
			cl = &clientLimiter{
				limiter: rate.NewLimiter(rate.Limit(cfg.RateLimit.RPS), cfg.RateLimit.Burst),
			}
			limiters[ip] = cl
		}
		cl.lastSeen = time.Now()
		mu.Unlock()

		if !cl.limiter.Allow() {
			// If request rate is extremely high (e.g., 5x burst), block the IP (DDoS protection)
			// This is a simple heuristic: if they keep hitting the limit while burst is exhausted.
			// For a real implementation, we'd track "violations" count.
			// Here we just use a catastrophic threshold if they try to burst way beyond allowed.
			if !cl.limiter.AllowN(time.Now(), cfg.RateLimit.Burst*10) {
				blockListMu.Lock()
				globalBlockList[ip] = time.Now().Add(cfg.DDoS.BlockDuration)
				blockListMu.Unlock()
			}

			c.Header("Retry-After", "1")
			respondError(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			c.Abort()
			return
		}

		c.Next()
	}
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// respondError writes a standard error JSON response.
func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: c.GetString("request_id"),
	})
}

// respondDomainError converts a domain error to an appropriate HTTP response.
func respondDomainError(c *gin.Context, err error) {
	status := domain.HTTPStatusFor(err)
	code := domain.ErrorCode(err)

	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		respondError(c, status, appErr.Code, appErr.Message)
		return
	}

	respondError(c, status, code, err.Error())
}
