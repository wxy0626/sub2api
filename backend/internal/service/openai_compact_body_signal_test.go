//go:build unit

package service

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHasCompactionTriggerInInput_DetectsCompactSignal(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"stream":true,
		"input":[
			{"type":"message","role":"user","content":"hello"},
			{"type":"compaction_trigger"}
		]
	}`)
	require.True(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_NoTrigger(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"input":[
			{"type":"message","role":"user","content":"hello"}
		]
	}`)
	require.False(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_EmptyInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[]}`)
	require.False(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_NoInputField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5"}`)
	require.False(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_EmptyBody(t *testing.T) {
	require.False(t, HasCompactionTriggerInInput(nil))
	require.False(t, HasCompactionTriggerInInput([]byte{}))
}

func TestHasCompactionTriggerInInput_StringInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":"compaction_trigger"}`)
	require.False(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_CompactTriggerOnly(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	require.True(t, HasCompactionTriggerInInput(body))
}

func TestIsOpenAIResponsesRemoteCompactionV2RequestRequiresAllSignals(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"compaction_trigger"}]}`)
	cases := []struct {
		name   string
		path   string
		header string
		want   bool
	}{
		{name: "all signals", path: "/v1/responses", header: "remote_compaction_v2", want: true},
		{name: "missing header", path: "/v1/responses", want: false},
		{name: "wrong path", path: "/v1/responses/compact", header: "remote_compaction_v2", want: false},
		{name: "stream false", path: "/v1/responses", header: "remote_compaction_v2", want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := body
			if tt.name == "stream false" {
				reqBody = []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"compaction_trigger"}]}`)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(reqBody))
			if tt.header != "" {
				c.Request.Header.Set("x-codex-beta-features", tt.header)
			}
			require.Equal(t, tt.want, IsOpenAIResponsesRemoteCompactionV2Request(c, reqBody))
		})
	}
}
