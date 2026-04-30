package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/spidey/notification-service/internal/domain"
	"github.com/spidey/notification-service/internal/repository"
)

type APIKeyService struct {
	repo *repository.APIKeyRepository
}

func NewAPIKeyService(repo *repository.APIKeyRepository) *APIKeyService {
	return &APIKeyService{repo: repo}
}

type CreateAPIKeyResult struct {
	Key    *domain.APIKey `json:"key"`
	APIKey string         `json:"api_key"`
}

func (s *APIKeyService) Create(ctx context.Context, name string) (*CreateAPIKeyResult, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name is required")
	}

	// 32 random bytes -> base32 string without padding, readable and URL-safe enough for headers.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("read random: %w", err)
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	prefix := strings.ToLower(enc[:10])
	apiKey := fmt.Sprintf("nh_%s_%s", prefix, enc)
	hash := sha256.Sum256([]byte(apiKey))

	k := &domain.APIKey{
		ID:        uuid.New().String(),
		Name:      name,
		Prefix:    prefix,
		CreatedAt: time.Now(),
	}
	if err := s.repo.Create(ctx, k, prefix, hash[:]); err != nil {
		return nil, err
	}
	return &CreateAPIKeyResult{Key: k, APIKey: apiKey}, nil
}

func (s *APIKeyService) List(ctx context.Context) ([]*domain.APIKey, error) {
	return s.repo.List(ctx)
}

func (s *APIKeyService) Revoke(ctx context.Context, id string) error {
	return s.repo.Revoke(ctx, id)
}

// VerifyAPIKey matches handler.AuthAPIKeyVerifier.
func (s *APIKeyService) VerifyAPIKey(c *gin.Context, apiKey string) (callerID string, ok bool, err error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", false, nil
	}
	// Expect format "nh_<prefix>_<rest>".
	parts := strings.Split(apiKey, "_")
	if len(parts) < 3 || parts[0] != "nh" {
		return "", false, nil
	}
	prefix := parts[1]
	hash := sha256.Sum256([]byte(apiKey))
	k, ok, err := s.repo.Verify(c.Request.Context(), prefix, hash[:])
	if err != nil {
		return "", false, err
	}
	if !ok || k == nil {
		return "", false, nil
	}
	return "api-key:" + k.ID, true, nil
}
