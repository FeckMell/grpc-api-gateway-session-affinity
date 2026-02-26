package service

import (
	"errors"
	"fmt"
)

const (
	ErrInternalServerError   = "internal_server_error"
	ErrBadParameter          = "bad_parameter"
	ErrEntityNotFound        = "entity_not_found"
	ErrInvalidUserOrPassword = "invalid_user_or_password"
)

type AuthError struct {
	Code    string
	Message string
	Inner   error
}

func (e AuthError) Error() string {
	if e.Inner != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Inner)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e AuthError) Unwrap() error { return e.Inner }

func NewEntityNotFoundError(message string, inner error) AuthError {
	return AuthError{Code: ErrEntityNotFound, Message: message, Inner: inner}
}

func IsEntityNotFound(err error) bool {
	var e AuthError
	return errors.As(err, &e) && e.Code == ErrEntityNotFound
}

func NewInvalidUserOrPasswordError(message string, inner error) AuthError {
	return AuthError{Code: ErrInvalidUserOrPassword, Message: message, Inner: inner}
}

func IsInvalidUserOrPassword(err error) bool {
	var e AuthError
	return errors.As(err, &e) && e.Code == ErrInvalidUserOrPassword
}

func NewInternalServerError(message string, inner error) AuthError {
	return AuthError{Code: ErrInternalServerError, Message: message, Inner: inner}
}

func IsInternalServerError(err error) bool {
	var e AuthError
	return errors.As(err, &e) && e.Code == ErrInternalServerError
}

func NewBadParameterError(message string, inner error) AuthError {
	return AuthError{Code: ErrBadParameter, Message: message, Inner: inner}
}

func IsBadParameter(err error) bool {
	var e AuthError
	return errors.As(err, &e) && e.Code == ErrBadParameter
}
