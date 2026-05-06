package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"
	clerkuser "github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/gin-gonic/gin"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
)

// ClerkClaimsFromContext extracts the claims map from the gin context.
// This is a compatibility helper for existing code that uses c.Get("claims").
func ClerkClaimsFromContext(c *gin.Context) map[string]interface{} {
	claimsAny, exists := c.Get("claims")
	if !exists {
		return nil
	}
	claims, ok := claimsAny.(map[string]interface{})
	if !ok {
		return nil
	}
	return claims
}

// extractRole extracts the role from Clerk claims.
// In Clerk, user metadata is stored in Custom claims.
func extractRole(claims *clerk.SessionClaims) string {
	// Try to get role from Custom claims (set via Clerk JWT Templates or public_metadata)
	if claims.Custom != nil {
		if m, ok := claims.Custom.(map[string]interface{}); ok {
			if role, ok := m["role"].(string); ok && role != "" {
				return role
			}
		}
	}
	return ""
}

// syncClerkUser fetches user details from the Clerk API and upserts them into
// our database. Returns the user's role. Falls back to UserRoleDev on any error.
// This is called on first login when the user isn't yet in our DB (e.g. when
// the Clerk webhook is not configured for local dev).
func syncClerkUser(ctx context.Context, clerkID string, repo *repository.UserRepository) domain.UserRole {
	clerkU, err := clerkuser.Get(ctx, clerkID)
	if err != nil {
		return domain.UserRoleDev
	}

	// Extract role from publicMetadata.
	role := domain.UserRoleDev
	if len(clerkU.PublicMetadata) > 0 {
		var meta map[string]interface{}
		if json.Unmarshal(clerkU.PublicMetadata, &meta) == nil {
			if r, ok := meta["role"].(string); ok && r != "" {
				role = domain.UserRole(r)
			}
		}
	}

	// Build email and name from Clerk user data.
	email := ""
	for _, e := range clerkU.EmailAddresses {
		if clerkU.PrimaryEmailAddressID != nil && e.ID == *clerkU.PrimaryEmailAddressID {
			email = e.EmailAddress
			break
		}
	}
	if email == "" && len(clerkU.EmailAddresses) > 0 {
		email = clerkU.EmailAddresses[0].EmailAddress
	}

	name := ""
	parts := []string{}
	if clerkU.FirstName != nil && *clerkU.FirstName != "" {
		parts = append(parts, *clerkU.FirstName)
	}
	if clerkU.LastName != nil && *clerkU.LastName != "" {
		parts = append(parts, *clerkU.LastName)
	}
	name = strings.Join(parts, " ")
	if name == "" && clerkU.Username != nil {
		name = *clerkU.Username
	}
	if name == "" {
		name = email
	}

	_, _ = repo.CreateFromClerk(ctx, clerkID, email, name, role)
	return role
}

// ClerkAuthenticator handles Clerk authentication logic.
type ClerkAuthenticator struct{}

// NewClerkAuthenticator creates a new ClerkAuthenticator.
func NewClerkAuthenticator() *ClerkAuthenticator {
	return &ClerkAuthenticator{}
}

// VerifyToken verifies a Clerk JWT token.
func (a *ClerkAuthenticator) VerifyToken(tokenStr string) (map[string]interface{}, error) {
	if tokenStr == "" {
		return nil, fmt.Errorf("empty token provided")
	}

	claims, err := clerkjwt.Verify(context.Background(), &clerkjwt.VerifyParams{Token: tokenStr})
	if err == nil && claims != nil {
		return map[string]interface{}{
			"sub":  claims.Subject,
			"sid":  claims.SessionID,
			"role": extractRole(claims),
		}, nil
	}

	return nil, fmt.Errorf("failed to verify Clerk token: %w", err)
}
