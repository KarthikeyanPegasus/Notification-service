package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spidey/notification-service/internal/service"
)

type AuthHandler struct {
	auth *service.AuthService
}

func NewAuthHandler(auth *service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	token, user, err := h.auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  user,
	})
}
