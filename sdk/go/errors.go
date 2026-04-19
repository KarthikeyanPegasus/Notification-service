package notification

import "fmt"

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	HTTPStatus int   `json:"-"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("notification api error %d: [%s] %s (request_id=%s)", e.HTTPStatus, e.Code, e.Message, e.RequestID)
}
