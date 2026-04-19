package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
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

// RateLimiter applies per-IP rate limiting using token bucket.
func RateLimiter(rps float64, burst int) gin.HandlerFunc {
	limiters := make(map[string]*rate.Limiter)
	mu := make(chan struct{}, 1)
	mu <- struct{}{}

	return func(c *gin.Context) {
		ip := c.ClientIP()

		<-mu
		limiter, ok := limiters[ip]
		if !ok {
			limiter = rate.NewLimiter(rate.Limit(rps), burst)
			limiters[ip] = limiter
		}
		mu <- struct{}{}

		if !limiter.Allow() {
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
