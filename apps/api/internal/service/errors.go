// Package service hosts the application use cases. ApplicationError carries
// the HTTP status and stable error code rendered by the transport layer.
package service

import "fmt"

type ApplicationError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *ApplicationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewError(statusCode int, code, message string) *ApplicationError {
	return &ApplicationError{StatusCode: statusCode, Code: code, Message: message}
}

func AsApplicationError(err error) (*ApplicationError, bool) {
	appErr, ok := err.(*ApplicationError)
	return appErr, ok
}

// Shared error constructors keep codes consistent across services. Each call
// returns a fresh value so callers may mutate without cross-request races.
func ErrInvalidCDK() *ApplicationError  { return NewError(422, "INVALID_CDK", "CDK format is invalid.") }
func ErrCDKNotFound() *ApplicationError { return NewError(404, "CDK_NOT_FOUND", "CDK was not found.") }
func ErrCDKDisabled() *ApplicationError { return NewError(403, "CDK_DISABLED", "CDK is disabled.") }
func ErrCDKActivationExpiry() *ApplicationError {
	return NewError(403, "CDK_ACTIVATION_EXPIRED", "CDK activation deadline has passed.")
}
func ErrCDKExpired() *ApplicationError {
	return NewError(403, "CDK_EXPIRED", "CDK service has expired.")
}
func ErrCDKExhausted() *ApplicationError {
	return NewError(402, "CDK_EXHAUSTED", "CDK has no remaining quota.")
}
func ErrCDKUnavailable() *ApplicationError {
	return NewError(409, "CDK_UNAVAILABLE", "CDK cannot be activated.")
}
func ErrCDKBindingInvalid() *ApplicationError {
	return NewError(409, "CDK_BINDING_INVALID", "CDK binding is invalid.")
}
func ErrAccountDisabled() *ApplicationError {
	return NewError(403, "ACCOUNT_DISABLED", "User account is disabled.")
}
func ErrSessionRequired() *ApplicationError {
	return NewError(401, "SESSION_INVALID", "User session is required.")
}
func ErrSessionInvalid() *ApplicationError {
	return NewError(401, "SESSION_INVALID", "User session is invalid or expired.")
}
func ErrAPIKeyNotFound() *ApplicationError {
	return NewError(404, "API_KEY_NOT_FOUND", "API Key was not found.")
}
func ErrAPIKeyDeleted() *ApplicationError {
	return NewError(409, "API_KEY_DELETED", "Deleted API Keys cannot be changed.")
}
func ErrAPIKeyDisabled() *ApplicationError {
	return NewError(403, "API_KEY_DISABLED", "API Key is disabled.")
}
func ErrAPIKeyInvalid() *ApplicationError {
	return NewError(401, "API_KEY_INVALID", "API Key is invalid.")
}
func ErrServiceUnavailable() *ApplicationError {
	return NewError(403, "SERVICE_UNAVAILABLE", "CDK service is unavailable.")
}
func ErrQuotaExhausted() *ApplicationError {
	return NewError(402, "QUOTA_EXHAUSTED", "No remaining quota.")
}
func ErrInvalidRequest() *ApplicationError {
	return NewError(422, "INVALID_REQUEST", "Request validation failed.")
}
func ErrAdminAuthFailed() *ApplicationError {
	return NewError(401, "ADMIN_AUTH_FAILED", "Administrator credentials are invalid.")
}
func ErrAdminSessionInvalid() *ApplicationError {
	return NewError(401, "ADMIN_SESSION_INVALID", "Administrator session is invalid or expired.")
}
func ErrAdminForbidden() *ApplicationError {
	return NewError(403, "ADMIN_FORBIDDEN", "Administrator role cannot perform this action.")
}
func ErrUserNotFound() *ApplicationError {
	return NewError(404, "USER_NOT_FOUND", "User was not found.")
}
func ErrKeyQuotaExhausted() *ApplicationError {
	return NewError(402, "KEY_QUOTA_EXHAUSTED", "该 API Key 的额度上限已用完，请调整限额或更换 Key。")
}

func ErrKeyIPForbidden() *ApplicationError {
	return NewError(403, "KEY_IP_FORBIDDEN", "调用来源 IP 不在该 Key 的白名单内。")
}

func ErrNotFound() *ApplicationError {
	return NewError(404, "NOT_FOUND", "Resource was not found.")
}
