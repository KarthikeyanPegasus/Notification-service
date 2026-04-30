package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/spidey/notification-service/internal/repository"
)

// RequireClientScope enforces/derives an api_key_id scope and stores it on context as "scoped_api_key_id".
// - admin: may omit api_key_id (context value will be empty)
// - api_key: forced to own key id
// - manager/dev/support: must provide api_key_id and be assigned to it
func RequireClientScope(users *repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKeyUUID, ok := enforceAPIKeyScope(c, users)
		if !ok {
			c.Abort()
			return
		}
		// For non-admin roles, enforceAPIKeyScope requires an explicit api_key_id already.
		if apiKeyUUID != nil {
			c.Set("scoped_api_key_id", apiKeyUUID.String())
		} else {
			c.Set("scoped_api_key_id", "")
		}
		c.Next()
	}
}

