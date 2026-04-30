package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type UserAdminHandler struct {
	users   *repository.UserRepository
	apiKeys *repository.APIKeyRepository
}

func NewUserAdminHandler(users *repository.UserRepository, apiKeys *repository.APIKeyRepository) *UserAdminHandler {
	return &UserAdminHandler{users: users, apiKeys: apiKeys}
}

func (h *UserAdminHandler) List(c *gin.Context) {
	out, err := h.users.List(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list users")
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *UserAdminHandler) Create(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Name     string `json:"name" binding:"required"`
		Password string `json:"password" binding:"required"`
		Role     string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	role := domain.UserRole(req.Role)
	if role != domain.UserRoleAdmin && role != domain.UserRoleManager && role != domain.UserRoleDev && role != domain.UserRoleSupport {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid role")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to hash password")
		return
	}
	u, err := h.users.Create(c.Request.Context(), req.Email, req.Name, hash, role)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create user")
		return
	}
	c.JSON(http.StatusCreated, u)
}

func (h *UserAdminHandler) SetRole(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid id")
		return
	}
	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	role := domain.UserRole(req.Role)
	if role != domain.UserRoleAdmin && role != domain.UserRoleManager && role != domain.UserRoleDev && role != domain.UserRoleSupport {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid role")
		return
	}
	if err := h.users.SetRole(c.Request.Context(), id, role); err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to set role")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *UserAdminHandler) SetAssignments(c *gin.Context) {
	userID := c.Param("id")
	if _, err := uuid.Parse(userID); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid id")
		return
	}
	var req struct {
		APIKeyIDs []string `json:"api_key_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	// Basic validate IDs
	for _, id := range req.APIKeyIDs {
		if _, err := uuid.Parse(id); err != nil {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid api_key_id")
			return
		}
	}
	if err := h.users.SetUserAPIKeys(c.Request.Context(), userID, req.APIKeyIDs); err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to set assignments")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *UserAdminHandler) ListAssignments(c *gin.Context) {
	userID := c.Param("id")
	if _, err := uuid.Parse(userID); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid id")
		return
	}
	ids, err := h.users.ListAPIKeysForUser(c.Request.Context(), userID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list assignments")
		return
	}
	c.JSON(http.StatusOK, gin.H{"api_key_ids": ids})
}

func (h *UserAdminHandler) Delete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid id")
		return
	}
	u, err := h.users.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == domain.ErrNotFound {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load user")
		return
	}
	if u.Role == domain.UserRoleAdmin {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "cannot delete admin user")
		return
	}
	if err := h.users.Delete(c.Request.Context(), id); err != nil {
		if err == domain.ErrNotFound {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		if err == sql.ErrNoRows {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete user")
		return
	}
	c.Status(http.StatusNoContent)
}
