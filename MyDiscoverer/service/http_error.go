package service

import (
	"errors"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/labstack/echo/v4"
)

// RegisterErrorHandler register custom error handler.
func RegisterErrorHandler(e *echo.Echo, logger log.Logger) {
	e.HTTPErrorHandler = NewHTTPErrorHandler(NewErrorCodeToStatusCodeMaps(), logger).Handler
}

// NewErrorCodeToStatusCodeMaps creates an error code to http status mapping.
func NewErrorCodeToStatusCodeMaps() map[string]int {
	var errorCodeToStatusCodeMaps = make(map[string]int)
	errorCodeToStatusCodeMaps[ErrBadParameter] = http.StatusBadRequest
	errorCodeToStatusCodeMaps[ErrEntityNotFound] = http.StatusNotFound
	errorCodeToStatusCodeMaps[ErrInternalServerError] = http.StatusInternalServerError

	return errorCodeToStatusCodeMaps
}

// HTTPErrorHandler is an error handler.
type HTTPErrorHandler struct {
	errorCodeToHTTPStatusCodeMap map[string]int
	logger                       log.Logger
}

// NewHTTPErrorHandler creates a new instance of the HTTPErrorHandler.
func NewHTTPErrorHandler(errorCodeToStatusCodeMaps map[string]int, logger log.Logger) *HTTPErrorHandler {
	return &HTTPErrorHandler{
		errorCodeToHTTPStatusCodeMap: errorCodeToStatusCodeMaps,
		logger:                       logger,
	}
}

func (h *HTTPErrorHandler) getStatusCode(errorCode string) int {
	status, ok := h.errorCodeToHTTPStatusCodeMap[errorCode]
	if ok {
		return status
	}

	return http.StatusInternalServerError
}

// Handler handles error returned by echo Handlers.
func (h *HTTPErrorHandler) Handler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	myErr := ToMyError(err)
	if myErr == nil {
		myErr = NewMyError(ErrInternalServerError, "an internal server error has occurred", err)
	}

	var statusCode int
	var he *echo.HTTPError
	if he, _ = err.(*echo.HTTPError); he != nil {
		codeStr := ErrInternalServerError
		if he.Internal != nil {
			if herr, ok := he.Internal.(*echo.HTTPError); ok {
				he = herr
			}
			var requestError *openapi3filter.RequestError
			if errors.As(he.Internal, &requestError) {
				codeStr = ErrBadParameter
			}
		}

		m, _ := he.Message.(string)
		myErr = NewMyError(codeStr, m, err)
		statusCode = he.Code
	} else {
		statusCode = h.getStatusCode(myErr.Code)
	}

	level.Error(h.logger).Log(
		"msg", "HTTP request error",
		"err", err,
	)

	// Send response
	if !c.Response().Committed {
		if c.Request().Method == http.MethodHead && he != nil {
			_ = c.NoContent(he.Code)
		} else {
			_ = c.JSON(statusCode, ErrResponse{Error: myErr})
		}
	}
}

// ErrResponse from server.
type ErrResponse struct {
	Error *MyError `json:"error,omitempty"`
}
