package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
)

func getRoleAndSubject(c *gin.Context) (role string, sub string) {
	claimsAny, ok := c.Get("claims")
	if !ok {
		return "", ""
	}
	switch v := claimsAny.(type) {
	case jwt.MapClaims:
		if s, ok := v["role"].(string); ok {
			role = s
		}
		switch sv := v["sub"].(type) {
		case string:
			sub = sv
		}
	case map[string]interface{}:
		if s, ok := v["role"].(string); ok {
			role = s
		}
		if s, ok := v["sub"].(string); ok {
			sub = s
		}
	}
	return role, sub
}

// enforceAPIKeyScope determines the effective api_key_id scope for endpoints that require a single explicit scope.
//
// Behavior:
// - admin: optional api_key_id (nil means "all"/global)
// - api_key callers: forced to their own api key id
// - dev/support: api_key_id is required and must be assigned to them
func enforceAPIKeyScope(c *gin.Context, users *repository.UserRepository) (*uuid.UUID, bool) {
	role, sub := getRoleAndSubject(c)
	if role == "" {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "missing role")
		return nil, false
	}

	// API key callers are always scoped to their own key.
	if role == "api_key" {
		caller := c.GetString("caller_id")
		if strings.HasPrefix(caller, "api-key:") {
			raw := strings.TrimPrefix(caller, "api-key:")
			if parsed, err := uuid.Parse(raw); err == nil {
				return &parsed, true
			}
		}
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid api key caller")
		return nil, false
	}

	// Admin can view all.
	if role == string(domain.UserRoleAdmin) {
		if v := strings.TrimSpace(c.Query("api_key_id")); v != "" {
			parsed, err := uuid.Parse(v)
			if err != nil {
				respondError(c, http.StatusBadRequest, "INVALID_PARAM", "invalid api_key_id")
				return nil, false
			}
			return &parsed, true
		}
		return nil, true
	}

	// Non-admin must be scoped.
	raw := strings.TrimSpace(c.Query("api_key_id"))
	if raw == "" {
		respondError(c, http.StatusBadRequest, "INVALID_PARAM", "api_key_id is required for this role")
		return nil, false
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PARAM", "invalid api_key_id")
		return nil, false
	}
	if users == nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "user repository not configured")
		return nil, false
	}
	if sub == "" {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing subject")
		return nil, false
	}
	ids, err := users.ListAPIKeysForUser(c.Request.Context(), sub)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load assignments")
		return nil, false
	}
	for _, id := range ids {
		if id == parsed.String() {
			return &parsed, true
		}
	}
	respondError(c, http.StatusForbidden, "FORBIDDEN", "api_key_id not assigned to user")
	return nil, false
}

// enforceAPIKeyScopeOrAssigned allows non-admin roles to omit api_key_id to mean "all assigned clients".
//
// Behavior:
// - admin: optional api_key_id (nil,nil means "all")
// - api_key callers: forced single scope
// - manager/dev/support:
//   - if api_key_id provided: must be assigned, returns single scope
//   - if api_key_id omitted: returns all assigned scopes (may be empty -> forbidden)
func enforceAPIKeyScopeOrAssigned(c *gin.Context, users *repository.UserRepository) (*uuid.UUID, []uuid.UUID, bool) {
	role, sub := getRoleAndSubject(c)
	if role == "" {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "missing role")
		return nil, nil, false
	}

	// API key callers: single forced scope.
	if role == "api_key" {
		apiKeyUUID, ok := enforceAPIKeyScope(c, users)
		return apiKeyUUID, nil, ok
	}

	// Admin: optional query scope.
	if role == string(domain.UserRoleAdmin) {
		apiKeyUUID, ok := enforceAPIKeyScope(c, users)
		return apiKeyUUID, nil, ok
	}

	if users == nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "user repository not configured")
		return nil, nil, false
	}
	if sub == "" {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing subject")
		return nil, nil, false
	}

	raw := strings.TrimSpace(c.Query("api_key_id"))
	if raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			respondError(c, http.StatusBadRequest, "INVALID_PARAM", "invalid api_key_id")
			return nil, nil, false
		}
		ids, err := users.ListAPIKeysForUser(c.Request.Context(), sub)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load assignments")
			return nil, nil, false
		}
		for _, id := range ids {
			if id == parsed.String() {
				return &parsed, nil, true
			}
		}
		respondError(c, http.StatusForbidden, "FORBIDDEN", "api_key_id not assigned to user")
		return nil, nil, false
	}

	ids, err := users.ListAPIKeysForUser(c.Request.Context(), sub)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load assignments")
		return nil, nil, false
	}
	if len(ids) == 0 {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "no client assignments")
		return nil, nil, false
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, s := range ids {
		if parsed, err := uuid.Parse(s); err == nil {
			out = append(out, parsed)
		}
	}
	if len(out) == 0 {
		respondError(c, http.StatusForbidden, "FORBIDDEN", "no valid client assignments")
		return nil, nil, false
	}
	return nil, out, true
}

