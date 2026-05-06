package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
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

// CORS enforces an allowed-origins allowlist. Pass an empty slice to disallow
// all cross-origin requests (safe default); pass ["*"] only in local dev.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	wildcard := false
	for _, o := range allowedOrigins {
		if o == "*" {
			wildcard = true
		} else {
			allowed[o] = true
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if wildcard || allowed[origin] {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
			}
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Request-ID,Idempotency-Key,X-API-Key,X-Service-Token")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// AuthContext is a minimal interface for auth middlewares that need to verify API keys.
// It lives in handler to avoid repository/service import cycles.
type AuthAPIKeyVerifier interface {
	VerifyAPIKey(ctx *gin.Context, apiKey string) (callerID string, ok bool, err error)
}

// AnyAuth allows a request if either a valid JWT bearer token is present or a valid X-API-Key is present.
// In debug mode, it allows requests with no credentials (matching JWTAuth behavior).
// userRepo is optional: when provided, it is used to look up a user's role from the DB if the
// Clerk JWT does not carry a role claim (i.e. no JWT template is configured in Clerk).
func AnyAuth(jwtSecret string, isDebugMode bool, verifier AuthAPIKeyVerifier, userRepo ...*repository.UserRepository) gin.HandlerFunc {
	var repo *repository.UserRepository
	if len(userRepo) > 0 {
		repo = userRepo[0]
	}
	return func(c *gin.Context) {
		// Debug shortcut (same as JWTAuth)
		if isDebugMode && c.GetHeader("Authorization") == "" && c.GetHeader("X-API-Key") == "" {
			c.Set("claims", map[string]interface{}{"sub": "debug-admin", "role": "admin"})
			c.Set("caller_id", "debug-admin")
			c.Next()
			return
		}

		// Try Clerk JWT first (new auth mechanism)
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := clerkJWTFallback(tokenStr)
			if err == nil {
				sub, ok := claims["sub"].(string)
				if !ok || strings.TrimSpace(sub) == "" {
					respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token subject")
					c.Abort()
					return
				}
				// If role is missing from the JWT (no Clerk JWT template configured),
				// fall back to the role stored in our database.
				if claims["role"] == "" && repo != nil {
					if u, err := repo.GetByClerkID(c.Request.Context(), sub); err == nil && u != nil {
						claims["role"] = string(u.Role)
					} else {
						// User not in DB yet (webhook not configured) — fetch from Clerk API
						// and auto-create the record so the correct role is used.
						claims["role"] = string(syncClerkUser(c.Request.Context(), sub, repo))
					}
				}
				// Final safety net: ensure role is never empty for a verified Clerk user.
				if claims["role"] == "" {
					claims["role"] = string(domain.UserRoleDev)
				}
				c.Set("claims", claims)
				c.Set("caller_id", sub)
				c.Next()
				return
			}
		}

		// Try HMAC JWT second (legacy behavior for backward compatibility)
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
				tokenStr := parts[1]
				token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
					if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
						return nil, jwt.ErrSignatureInvalid
					}
					return []byte(jwtSecret), nil
				})
				if err == nil && token != nil && token.Valid {
					if claims, ok := token.Claims.(jwt.MapClaims); ok {
						c.Set("claims", claims)
						if sub, ok := claims["sub"].(string); ok {
							c.Set("caller_id", sub)
						}
					}
					c.Next()
					return
				}
			}
		}

		// Fall back to API key
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing Authorization header or X-API-Key")
			c.Abort()
			return
		}
		if verifier == nil {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "api key auth not configured")
			c.Abort()
			return
		}
		callerID, ok, err := verifier.VerifyAPIKey(c, apiKey)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "api key verification failed")
			c.Abort()
			return
		}
		if !ok {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid API key")
			c.Abort()
			return
		}
		c.Set("claims", map[string]interface{}{"sub": callerID, "role": "api_key"})
		c.Set("caller_id", callerID)
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

// RequireRole enforces that the JWT claim `role` matches one of the allowed roles.
func RequireRole(allowed ...string) gin.HandlerFunc {
	allowedSet := map[string]struct{}{}
	for _, r := range allowed {
		allowedSet[r] = struct{}{}
	}
	return func(c *gin.Context) {
		claimsAny, ok := c.Get("claims")
		if !ok {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing auth claims")
			c.Abort()
			return
		}
		role := ""
		switch v := claimsAny.(type) {
		case jwt.MapClaims:
			if s, ok := v["role"].(string); ok {
				role = s
			}
		case map[string]interface{}:
			if s, ok := v["role"].(string); ok {
				role = s
			}
		}
		if role == "" {
			respondError(c, http.StatusForbidden, "FORBIDDEN", "missing role")
			c.Abort()
			return
		}
		if _, ok := allowedSet[role]; !ok {
			respondError(c, http.StatusForbidden, "FORBIDDEN", "insufficient role")
			c.Abort()
			return
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
	limiter     *rate.Limiter
	lastSeen    time.Time
	deniedCount int
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
			mu.Lock()
			cl.deniedCount++
			shouldBlock := cfg.DDoS.BlockThreshold > 0 && cl.deniedCount >= cfg.DDoS.BlockThreshold
			if shouldBlock {
				cl.deniedCount = 0
			}
			mu.Unlock()

			if shouldBlock {
				blockUntil := time.Now().Add(cfg.DDoS.BlockDuration)
				blockListMu.Lock()
				globalBlockList[ip] = blockUntil
				blockListMu.Unlock()
				c.Header("X-Security-Action", "Blocked")
				respondError(c, http.StatusForbidden, "ACCESS_DENIED", "your IP has been temporarily blocked due to excessive requests")
				c.Abort()
				return
			}
			c.Header("Retry-After", "1")
			respondError(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			c.Abort()
			return
		}

		// Successful request resets abuse counter.
		mu.Lock()
		cl.deniedCount = 0
		mu.Unlock()
		c.Next()
	}
}

// RequireOwnershipOrRole checks that the caller owns the resource identified by :user_id
// or has one of the specified elevated roles. The caller's identity is extracted from
// JWT claims (sub) or the debug-admin fallback.
func requireOwnershipOrRole(allowedRoles ...string) gin.HandlerFunc {
	allowedSet := make(map[string]struct{}, len(allowedRoles))
	for _, r := range allowedRoles {
		allowedSet[r] = struct{}{}
	}
	return func(c *gin.Context) {
		userID := c.Param("user_id")
		if userID == "" {
			c.Next()
			return
		}

		claimsAny, ok := c.Get("claims")
		if !ok {
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing auth claims")
			c.Abort()
			return
		}

		// Extract caller sub and role
		var callerSub string
		var callerRole string
		switch v := claimsAny.(type) {
		case jwt.MapClaims:
			if s, ok := v["sub"].(string); ok {
				callerSub = s
			}
			if r, ok := v["role"].(string); ok {
				callerRole = r
			}
		case map[string]interface{}:
			if s, ok := v["sub"].(string); ok {
				callerSub = s
			}
			if r, ok := v["role"].(string); ok {
				callerRole = r
			}
		}

		// Allow if caller has one of the elevated roles (admin, support)
		if callerRole != "" {
			if _, elevated := allowedSet[callerRole]; elevated {
				c.Next()
				return
			}
		}

		// Allow if caller owns the resource (sub matches user_id)
		if callerSub != "" && callerSub == userID {
			c.Next()
			return
		}

		// Debug admin bypass
		if callerSub == "debug-admin" {
			c.Next()
			return
		}

		respondError(c, http.StatusForbidden, "FORBIDDEN", "access denied: you can only access your own preferences")
		c.Abort()
	}
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// respondError writes a standard error JSON response.

// clerkJWTFallback attempts to verify a token as a Clerk JWT first,
// then falls back to the old HMAC-signed JWT for backward compatibility.
//
// During the migration period, both Clerk-issued tokens and legacy
// HMAC tokens are accepted.
func clerkJWTFallback(tokenStr string) (map[string]interface{}, error) {
	if tokenStr == "" {
		return nil, errors.New("empty token provided")
	}
	claims, err := clerkjwt.Verify(context.Background(), &clerkjwt.VerifyParams{Token: tokenStr})
	if err == nil && claims != nil {
		return map[string]interface{}{
			"sub":  claims.Subject,
			"sid":  claims.SessionID,
			"role": extractRole(claims),
		}, nil
	}
	return nil, err
}

// internalOnly restricts an endpoint to requests originating from localhost or
// RFC 1918 private address ranges. Use this for observability endpoints like /metrics
// that must not be exposed on the public-facing interface.
func internalOnly() gin.HandlerFunc {
	privateNets := func() []*net.IPNet {
		cidrs := []string{
			"127.0.0.0/8",    // loopback IPv4
			"::1/128",        // loopback IPv6
			"10.0.0.0/8",     // RFC 1918
			"172.16.0.0/12",  // RFC 1918
			"192.168.0.0/16", // RFC 1918
			"fc00::/7",       // IPv6 ULA
		}
		nets := make([]*net.IPNet, 0, len(cidrs))
		for _, cidr := range cidrs {
			_, n, _ := net.ParseCIDR(cidr)
			nets = append(nets, n)
		}
		return nets
	}()

	return func(c *gin.Context) {
		ip := net.ParseIP(c.ClientIP())
		if ip == nil {
			respondError(c, http.StatusForbidden, "FORBIDDEN", "unable to determine client IP")
			c.Abort()
			return
		}
		for _, n := range privateNets {
			if n.Contains(ip) {
				c.Next()
				return
			}
		}
		respondError(c, http.StatusForbidden, "FORBIDDEN", "access restricted to internal network")
		c.Abort()
	}
}

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

	// Sanitize 500 errors to prevent information leakage
	message := err.Error()
	if status >= 500 {
		// Log the full error internally for debugging
		requestID := c.GetString("request_id")
		if logger, ok := c.Get("logger"); ok {
			if zapLogger, ok := logger.(*zap.Logger); ok {
				zapLogger.Error("internal server error",
					zap.String("request_id", requestID),
					zap.Error(err),
				)
			}
		}
		// Return generic message to client
		message = "an unexpected error occurred"
	}

	respondError(c, status, code, message)
}
