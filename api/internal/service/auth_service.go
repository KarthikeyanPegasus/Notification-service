package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	cfg  *config.Config
	repo *repository.UserRepository
}

func NewAuthService(cfg *config.Config, repo *repository.UserRepository) *AuthService {
	return &AuthService{cfg: cfg, repo: repo}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (token string, user *domain.User, err error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return "", nil, fmt.Errorf("email and password are required")
	}
	row, err := s.repo.GetByEmailWithHash(ctx, email)
	if err != nil {
		return "", nil, fmt.Errorf("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword(row.PasswordHash, []byte(password)); err != nil {
		return "", nil, fmt.Errorf("invalid credentials")
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  row.User.ID,
		"role": string(row.User.Role),
		"exp":  time.Now().Add(s.cfg.JWT.Expiry).Unix(),
	})
	signed, err := tok.SignedString([]byte(s.cfg.JWT.Secret))
	if err != nil {
		return "", nil, fmt.Errorf("sign token: %w", err)
	}
	return signed, &row.User, nil
}
