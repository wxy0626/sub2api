package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountCredentialRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.GET("/api/v1/admin/accounts/:id/credentials/:key", handler.GetCredential)
	return router, adminSvc
}

func TestAccountHandlerGetCredential(t *testing.T) {
	t.Run("returns credential value", func(t *testing.T) {
		router, adminSvc := setupAccountCredentialRouter()
		adminSvc.getAccountCredentialResult = "sk-secret"

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/42/credentials/api_key", nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var payload struct {
			Data struct {
				Value string `json:"value"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		require.Equal(t, "sk-secret", payload.Data.Value)
	})

	t.Run("rejects invalid account id", func(t *testing.T) {
		router, _ := setupAccountCredentialRouter()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/not-a-number/credentials/api_key", nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("propagates service error", func(t *testing.T) {
		router, adminSvc := setupAccountCredentialRouter()
		adminSvc.getAccountCredentialErr = infraerrors.NotFound("credential_not_found", "credential not found")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/42/credentials/api_key", nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("propagates unexpected service error", func(t *testing.T) {
		router, adminSvc := setupAccountCredentialRouter()
		adminSvc.getAccountCredentialErr = errors.New("db connection failed")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/42/credentials/api_key", nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
