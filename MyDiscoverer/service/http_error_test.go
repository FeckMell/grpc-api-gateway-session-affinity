package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-kit/log"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewErrorCodeToStatusCodeMaps(t *testing.T) {
	m := NewErrorCodeToStatusCodeMaps()
	require.NotNil(t, m)
	assert.Equal(t, http.StatusBadRequest, m[ErrBadParameter])
	assert.Equal(t, http.StatusNotFound, m[ErrEntityNotFound])
	assert.Equal(t, http.StatusInternalServerError, m[ErrInternalServerError])
}

func TestHTTPErrorHandler_Handler_MyError_ReturnsMappedStatus(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := NewHTTPErrorHandler(NewErrorCodeToStatusCodeMaps(), log.NewNopLogger())
	err := NewBadParameterError("invalid body", nil)
	handler.Handler(err, c)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var body ErrResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.NotNil(t, body.Error)
	assert.Equal(t, ErrBadParameter, body.Error.Code)
	assert.Equal(t, "invalid body", body.Error.Message)
}

func TestHTTPErrorHandler_Handler_NonMyError_Returns500(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := NewHTTPErrorHandler(NewErrorCodeToStatusCodeMaps(), log.NewNopLogger())
	err := assert.AnError
	handler.Handler(err, c)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var body ErrResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.NotNil(t, body.Error)
	assert.Equal(t, ErrInternalServerError, body.Error.Code)
}

func TestHTTPErrorHandler_Handler_EchoHTTPError_WithRequestError_ReturnsBadParameter(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := NewHTTPErrorHandler(NewErrorCodeToStatusCodeMaps(), log.NewNopLogger())
	reqErr := &openapi3filter.RequestError{Err: assert.AnError}
	he := echo.NewHTTPError(http.StatusBadRequest, "request body has an error")
	he.Internal = reqErr
	handler.Handler(he, c)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var body ErrResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.NotNil(t, body.Error)
	assert.Equal(t, ErrBadParameter, body.Error.Code)
}

func TestRegisterErrorHandler(t *testing.T) {
	e := echo.New()
	RegisterErrorHandler(e, log.NewNopLogger())
	require.NotNil(t, e.HTTPErrorHandler)
}
