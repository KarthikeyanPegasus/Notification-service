package domain

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for domain-level checks.
var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyExists     = errors.New("already exists")
	ErrAlreadyDelivered  = errors.New("notification already delivered")
	ErrAlreadyCancelled  = errors.New("notification already cancelled")
	ErrAlreadyRunning    = errors.New("notification delivery already started")
	ErrRateLimited       = errors.New("rate limit exceeded")
	ErrOptedOut          = errors.New("user has opted out of this channel")
	ErrInvalidRecipient  = errors.New("invalid recipient")
	ErrOTPExpired        = errors.New("otp has expired")
	ErrOTPInvalid        = errors.New("invalid otp")
	ErrTooManyAttempts   = errors.New("too many otp attempts")
	ErrAllProvidersOpen  = errors.New("all providers circuit-breaker open")
	ErrScheduleInPast    = errors.New("scheduled time is in the past")
	ErrConflict          = errors.New("conflict")
)

// AppError is a structured HTTP-mappable error.
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Err        error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Err }

func NewAppError(code, message string, status int, err error) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: status, Err: err}
}

// HTTPStatus returns the appropriate HTTP status code for a domain error.
func HTTPStatusFor(err error) int {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.HTTPStatus
	}

	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, ErrAlreadyDelivered),
		errors.Is(err, ErrAlreadyRunning),
		errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrRateLimited):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrOTPExpired):
		return http.StatusGone
	case errors.Is(err, ErrTooManyAttempts):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrOTPInvalid),
		errors.Is(err, ErrInvalidRecipient),
		errors.Is(err, ErrScheduleInPast):
		return http.StatusBadRequest
	case errors.Is(err, ErrOptedOut):
		return http.StatusUnprocessableEntity
	}
	return http.StatusInternalServerError
}

// ErrorCode maps domain errors to string codes for API responses.
func ErrorCode(err error) string {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}

	switch {
	case errors.Is(err, ErrNotFound):
		return "NOT_FOUND"
	case errors.Is(err, ErrAlreadyExists):
		return "ALREADY_EXISTS"
	case errors.Is(err, ErrAlreadyDelivered):
		return "ALREADY_DELIVERED"
	case errors.Is(err, ErrAlreadyRunning):
		return "ALREADY_RUNNING"
	case errors.Is(err, ErrAlreadyCancelled):
		return "ALREADY_CANCELLED"
	case errors.Is(err, ErrRateLimited):
		return "RATE_LIMITED"
	case errors.Is(err, ErrOptedOut):
		return "OPTED_OUT"
	case errors.Is(err, ErrOTPExpired):
		return "OTP_EXPIRED"
	case errors.Is(err, ErrOTPInvalid):
		return "INVALID_OTP"
	case errors.Is(err, ErrTooManyAttempts):
		return "TOO_MANY_ATTEMPTS"
	case errors.Is(err, ErrScheduleInPast):
		return "SCHEDULE_IN_PAST"
	case errors.Is(err, ErrAllProvidersOpen):
		return "ALL_PROVIDERS_UNAVAILABLE"
	}
	return "INTERNAL_ERROR"
}
