package errors

import (
	"errors"
	"fmt"
)

// Code represents a stable error code for classification.
type Code string

const (
	CodeConfigInvalid      Code = "E_CONFIG_INVALID"
	CodeRouteConflict      Code = "E_ROUTE_CONFLICT"
	CodePathRejected       Code = "E_PATH_REJECTED"
	CodeScriptNotAllowed   Code = "E_SCRIPT_NOT_ALLOWED"
	CodeRuntimeUnavailable Code = "E_RUNTIME_UNAVAILABLE"
	CodeRuntimeUnhealthy   Code = "E_RUNTIME_UNHEALTHY"
	CodePHPQueueFull       Code = "E_PHP_QUEUE_FULL"
	CodePHPTimeout         Code = "E_PHP_TIMEOUT"
	CodeFastCGIProtocol    Code = "E_FASTCGI_PROTOCOL"
	CodeResponseInvalid    Code = "E_RESPONSE_INVALID"
	CodeFileNotFound       Code = "E_FILE_NOT_FOUND"
	CodeAccessDenied       Code = "E_ACCESS_DENIED"
	CodeUpstreamFailed     Code = "E_UPSTREAM_FAILED"
)

// GatewayError is a structured error with a stable code and optional cause.
type GatewayError struct {
	Code    Code
	Message string
	Cause   error
}

func (e *GatewayError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *GatewayError) Unwrap() error {
	return e.Cause
}

// New creates a new GatewayError.
func New(code Code, message string) *GatewayError {
	return &GatewayError{Code: code, Message: message}
}

// Wrap creates a GatewayError wrapping a cause.
func Wrap(code Code, message string, cause error) *GatewayError {
	return &GatewayError{Code: code, Message: message, Cause: cause}
}

// IsCode checks if an error (or any error in its chain) has the given code.
func IsCode(err error, code Code) bool {
	var gw *GatewayError
	if errors.As(err, &gw) {
		return gw.Code == code
	}
	return false
}
