package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExplicitCodexThreadSessionID_PrefersChildThreadOverParentSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	body := []byte(`{
		"model":"flash",
		"prompt_cache_key":"parent-session",
		"client_metadata":{
			"session_id":"parent-session",
			"thread_id":"child-thread"
		}
	}`)

	svc := &OpenAIGatewayService{}
	require.Equal(t, "codex-thread:child-thread", svc.ExtractSessionID(c, body))
	require.Equal(t,
		xxhash.Sum64String("codex-thread:child-thread"),
		xxhash.Sum64String(svc.ExtractSessionID(c, body)),
	)
	require.Equal(t, "codex-thread:child-thread", explicitOpenAISessionID(c, body))
}

func TestExplicitCodexThreadSessionID_ReadsEmbeddedTurnMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set(
		"x-codex-turn-metadata",
		`{"session_id":"parent-session","thread_id":"header-thread"}`,
	)

	body := []byte(`{
		"model":"flash",
		"prompt_cache_key":"parent-session",
		"client_metadata":{
			"x-codex-turn-metadata":"{\"session_id\":\"parent-session\",\"thread_id\":\"body-thread\"}"
		}
	}`)

	require.Equal(t, "codex-thread:header-thread", explicitCodexThreadSessionID(c, body))
}

func TestExplicitCodexThreadSessionID_FallsBackToPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{}
	body := []byte(`{"model":"flash","prompt_cache_key":"parent-session"}`)
	require.Equal(t, "parent-session", svc.ExtractSessionID(c, body))
	require.Equal(t, "parent-session", explicitOpenAISessionID(c, body))
}
