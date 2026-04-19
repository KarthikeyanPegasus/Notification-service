package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/spidey/notification-service/internal/cache"
	"github.com/spidey/notification-service/internal/domain"
)

const (
	otpDefaultExpiry    = 5 * time.Minute
	otpMaxAttempts      = 5
	otpMaxPerHour       = 5
	otpGenerationWindow = time.Hour
)

// OTPService handles OTP generation, storage, and verification.
type OTPService struct {
	cache *cache.Client
}

func NewOTPService(cacheClient *cache.Client) *OTPService {
	return &OTPService{cache: cacheClient}
}

func otpKey(userID, purpose string) string {
	return fmt.Sprintf("otp:value:%s:%s", userID, purpose)
}

func otpAttemptsKey(userID, purpose string) string {
	return fmt.Sprintf("otp:attempts:%s:%s", userID, purpose)
}

func otpRateKey(userID, purpose string) string {
	return fmt.Sprintf("otp:rate:%s:%s", userID, purpose)
}

// Generate creates a cryptographically random 6-digit OTP, stores it in Redis, and returns it.
func (s *OTPService) Generate(ctx context.Context, userID, purpose string, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = otpDefaultExpiry
	}

	// Rate limiting: max 5 OTPs per user+purpose per hour
	rateKey := otpRateKey(userID, purpose)
	count, err := s.cache.Incr(ctx, rateKey)
	if err != nil {
		return "", fmt.Errorf("checking OTP rate limit: %w", err)
	}
	if count == 1 {
		_ = s.cache.Expire(ctx, rateKey, otpGenerationWindow)
	}
	if count > otpMaxPerHour {
		return "", fmt.Errorf("%w: max %d OTPs per hour per purpose", domain.ErrRateLimited, otpMaxPerHour)
	}

	otp, err := generateSecureOTP()
	if err != nil {
		return "", fmt.Errorf("generating OTP: %w", err)
	}

	key := otpKey(userID, purpose)
	if err := s.cache.SetString(ctx, key, otp, expiry); err != nil {
		return "", fmt.Errorf("storing OTP: %w", err)
	}

	// Reset attempt counter for this new OTP
	attemptsKey := otpAttemptsKey(userID, purpose)
	_ = s.cache.Del(ctx, attemptsKey)

	return otp, nil
}

// Verify checks a submitted OTP. Returns nil on success (and deletes the OTP for single-use).
func (s *OTPService) Verify(ctx context.Context, userID, purpose, inputOTP string) error {
	key := otpKey(userID, purpose)
	stored, err := s.cache.GetString(ctx, key)
	if err != nil {
		if errors.Is(err, cache.ErrCacheMiss) {
			return domain.ErrOTPExpired
		}
		return fmt.Errorf("retrieving OTP: %w", err)
	}

	// Track attempts before comparing (prevents timing oracle on attempt count)
	attemptsKey := otpAttemptsKey(userID, purpose)
	attempts, err := s.cache.Incr(ctx, attemptsKey)
	if err != nil {
		return fmt.Errorf("tracking OTP attempts: %w", err)
	}
	if attempts == 1 {
		// Set expiry on the attempts counter equal to the OTP TTL
		_ = s.cache.Expire(ctx, attemptsKey, otpDefaultExpiry)
	}
	if attempts > otpMaxAttempts {
		return domain.ErrTooManyAttempts
	}

	if stored != inputOTP {
		return domain.ErrOTPInvalid
	}

	// Single-use: delete on success
	_ = s.cache.Del(ctx, key, attemptsKey)
	return nil
}

// generateSecureOTP generates a 6-digit cryptographically random OTP.
func generateSecureOTP() (string, error) {
	max := big.NewInt(999999)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
