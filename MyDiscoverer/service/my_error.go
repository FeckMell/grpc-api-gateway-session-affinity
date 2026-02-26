package service

import (
	"errors"
	"fmt"
)

const (
	// ErrInternalServerError means that an internal server error has occurred.
	ErrInternalServerError = "internal_server_error"
	// ErrEntityNotFound means that record or row is absent in repository or storage.
	ErrEntityNotFound = "entity_not_found"
	// ErrBadParameter means that provided parameter does not match declared.
	ErrBadParameter = "bad_parameter"
)

// MyError represents an error within the context of mydiscoverer services.
type MyError struct {
	// Code is a machine-readable code.
	Code string `json:"code,omitempty"`
	// Message is a human-readable message.
	Message string `json:"message"`
	// Inner is a wrapped error that is never shown to API consumers.
	Inner error `json:"-"`
}

// NewMyError creates a new MyError.
func NewMyError(code string, message string, inner error) *MyError {
	return &MyError{
		Code:    code,
		Message: message,
		Inner:   inner,
	}
}

func NewInternalServerError(message string, inner error) *MyError {
	myInner := ToMyError(inner)
	if myInner != nil {
		return myInner
	}

	return NewMyError(ErrInternalServerError, message, inner)
}

func NewEntityNotFoundError(message string, inner error) *MyError {
	myInner := ToMyError(inner)
	if myInner != nil {
		return myInner
	}

	return NewMyError(ErrEntityNotFound, message, inner)
}

func NewBadParameterError(message string, inner error) *MyError {
	myInner := ToMyError(inner)
	if myInner != nil {
		return myInner
	}

	return NewMyError(ErrBadParameter, message, inner)
}

func (e MyError) Error() string {
	if e.Inner != nil {
		return fmt.Sprintf("%s %s: %v", e.Code, e.Message, e.Inner)
	}

	return fmt.Sprintf("%s %s", e.Code, e.Message)
}

// Unwrap the error returning the error's reason.
func (e MyError) Unwrap() error {
	return e.Inner
}

// ToMyError returns a pointer to a mydiscoverer error, or nil if it is not a mydiscoverer error.
func ToMyError(err error) *MyError {
	var e *MyError
	if errors.As(err, &e) {
		return e
	}

	return nil
}

// ToMyErrorCode returns the code of the error, if available.
func ToMyErrorCode(err error) string {
	myerror := ToMyError(err)
	if myerror != nil {
		return myerror.Code
	}
	return ""
}

func IsMyError(err error, code string) bool {
	myerror := ToMyError(err)
	if myerror != nil {
		return myerror.Code == code
	}
	return false
}

func IsInternalServerError(err error) bool {
	return IsMyError(err, ErrInternalServerError)
}

func IsEntityNotFoundError(err error) bool {
	return IsMyError(err, ErrEntityNotFound)
}

func IsBadParameterError(err error) bool {
	return IsMyError(err, ErrBadParameter)
}
