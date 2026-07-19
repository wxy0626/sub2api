package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountHandlerUpdateTestModePersistsOpenAIExtraKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := newStubAdminService()
	stub.getAccountResult = &service.Account{
		ID:       9,
		Name:     "OpenAI account",
		Platform: service.PlatformOpenAI,
		Status:   service.StatusActive,
		Extra:    map[string]any{"other_setting": true},
	}
	handler := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.PUT("/accounts/:id/test-mode", handler.UpdateTestMode)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/accounts/9/test-mode", bytes.NewBufferString(`{"mode":"responses"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, map[string]any{accountTestModeExtraKey: "responses"}, stub.lastAccountExtraUpdate)
	var body struct {
		Data struct {
			ID    int64          `json:"id"`
			Extra map[string]any `json:"extra"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, int64(9), body.Data.ID)
	require.Equal(t, "responses", body.Data.Extra[accountTestModeExtraKey])
	require.Equal(t, true, body.Data.Extra["other_setting"])
}

func TestAccountHandlerUpdateTestModeRejectsInvalidModeAndNonOpenAIAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		platform string
		body     string
	}{
		{name: "invalid mode", platform: service.PlatformOpenAI, body: `{"mode":"chat"}`},
		{name: "non OpenAI account", platform: service.PlatformAnthropic, body: `{"mode":"responses"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newStubAdminService()
			stub.getAccountResult = &service.Account{ID: 10, Platform: tt.platform, Status: service.StatusActive}
			handler := NewAccountHandler(stub, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			router := gin.New()
			router.PUT("/accounts/:id/test-mode", handler.UpdateTestMode)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/accounts/10/test-mode", bytes.NewBufferString(tt.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), "保存模型测试模式失败")
			require.Zero(t, stub.updateAccountExtraCalls)
		})
	}
}
