//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stallAfterFirstDataUpstream 先回响应头（可带首个 SSE 数据块），随后阻塞读直到请求 ctx 取消。
// 用于模拟 napi 中转类上游"响应头到达后首包挂起 30-60 秒"的故障形态。
// release 供看门狗禁用场景手动放行（上游 ctx 为 WithoutCancel，Done() 永不关闭）。
type stallAfterFirstDataUpstream struct {
	firstData string
	release   chan struct{}
	canceled  chan struct{}
	once      sync.Once
}

type stallAfterFirstBody struct {
	ctx      context.Context
	data     string
	sent     bool
	upstream *stallAfterFirstDataUpstream
}

// blockUntilInterrupted 阻塞直到 ctx 取消或测试手动放行，返回相应的读错误。
func (b *stallAfterFirstBody) blockUntilInterrupted() error {
	markCanceled := func() {
		b.upstream.once.Do(func() { close(b.upstream.canceled) })
	}
	if b.upstream.release != nil {
		select {
		case <-b.ctx.Done():
			markCanceled()
			return b.ctx.Err()
		case <-b.upstream.release:
			return errors.New("test released upstream")
		}
	}
	<-b.ctx.Done()
	markCanceled()
	return b.ctx.Err()
}

func (b *stallAfterFirstBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		if b.data == "" {
			// 无首块数据：直接进入阻塞等待，模拟首包前挂起。
			return 0, b.blockUntilInterrupted()
		}
		return copy(p, b.data), nil
	}
	return 0, b.blockUntilInterrupted()
}

func (b *stallAfterFirstBody) Close() error { return nil }

func (u *stallAfterFirstDataUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_stall_after_headers"}},
		Body:       &stallAfterFirstBody{ctx: req.Context(), data: u.firstData, upstream: u},
	}, nil
}

func (u *stallAfterFirstDataUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, "", 0, 0)
}

// firstOutputTimeoutTestConfig 返回启用首输出看门狗（N 秒）的测试配置。
func firstOutputTimeoutTestConfig(seconds int) *config.Config {
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = seconds
	cfg.Gateway.MaxLineSize = defaultMaxLineSize
	return cfg
}

// translatedFirstOutputTestAccount 返回走 CC→Responses 翻译路径的 OAuth 测试账号。
func translatedFirstOutputTestAccount() *Account {
	return &Account{
		ID: 201, Name: "translated-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"},
	}
}

func requireFirstOutputTimeoutFailover(t *testing.T, err error, clientBody string) {
	t.Helper()
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "first_output_timeout")
	require.True(t, failoverErr.SafeToFailoverAfterWrite)
	require.Empty(t, clientBody, "看门狗换号前客户端必须 0 字节")
}

// 翻译路径 + 响应头阶段挂起：上游迟迟不回响应头，看门狗到点取消并转译为换号错误。
func TestChatCompletionsTranslatedFirstOutputTimeoutOnResponseHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &blockingOpenAIResponseHeaderUpstream{canceled: make(chan struct{})}
	svc := &OpenAIGatewayService{cfg: firstOutputTimeoutTestConfig(1), httpUpstream: upstream}
	body := []byte(`{"model":"gpt-5.5","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	started := time.Now()
	_, err := svc.ForwardAsChatCompletions(context.Background(), c, translatedFirstOutputTestAccount(), body, "", "")

	requireFirstOutputTimeoutFailover(t, err, rec.Body.String())
	require.Less(t, time.Since(started), 1300*time.Millisecond)
	select {
	case <-upstream.canceled:
	default:
		t.Fatal("response-header timeout did not cancel the translated-path upstream request")
	}
}

// raw 直通路径 + 首包阶段挂起：响应头已到但没有任何 SSE 数据，看门狗到点取消并换号。
func TestChatCompletionsRawFirstOutputTimeoutOnSemanticOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &stallAfterFirstDataUpstream{firstData: "", canceled: make(chan struct{})}
	svc := &OpenAIGatewayService{cfg: firstOutputTimeoutTestConfig(1), httpUpstream: upstream}
	body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	started := time.Now()
	_, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")

	requireFirstOutputTimeoutFailover(t, err, rec.Body.String())
	require.Less(t, time.Since(started), 1300*time.Millisecond)
	select {
	case <-upstream.canceled:
	default:
		t.Fatal("semantic-output timeout did not cancel the raw-path upstream request")
	}
}

// raw 直通路径 + 正常流：首个数据块在截止前到达时，看门狗停表，流正常收尾不受影响。
func TestChatCompletionsRawHealthyStreamNotKilledByWatchdog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_wd","object":"chat.completion.chunk","model":"gpt-5.6-sol","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		"",
		`data: {"id":"chatcmpl_wd","object":"chat.completion.chunk","model":"gpt-5.6-sol","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_watchdog_healthy"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: firstOutputTimeoutTestConfig(5), httpUpstream: upstream}
	body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), `"content":"ok"`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
}

// raw 直通路径非流式 + 缓冲读取期间挂起：看门狗触发后转译为换号错误。
func TestChatCompletionsRawBufferedFirstOutputTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &stallAfterFirstDataUpstream{firstData: "", canceled: make(chan struct{})}
	svc := &OpenAIGatewayService{cfg: firstOutputTimeoutTestConfig(1), httpUpstream: upstream}
	body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")

	requireFirstOutputTimeoutFailover(t, err, rec.Body.String())
	select {
	case <-upstream.canceled:
	default:
		t.Fatal("buffered-read timeout did not cancel the raw-path upstream request")
	}
}

// 看门狗关闭（0）时保持既有行为：挂起上游不会被看门狗取消，直到测试手动放行。
func TestChatCompletionsRawFirstOutputTimeoutDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &stallAfterFirstDataUpstream{firstData: "", canceled: make(chan struct{}), release: make(chan struct{})}
	svc := &OpenAIGatewayService{cfg: firstOutputTimeoutTestConfig(0), httpUpstream: upstream}
	body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")
	}()
	select {
	case <-done:
		t.Fatal("disabled watchdog must not abort the stalled upstream")
	case <-upstream.canceled:
		t.Fatal("disabled watchdog must not cancel the upstream request")
	case <-time.After(1500 * time.Millisecond):
		// 预期：1.5 秒后仍在等待上游，说明看门狗未介入。
	}
	// 收尾：手动放行让阻塞的读取退出，避免泄漏测试 goroutine。
	close(upstream.release)
	<-done
	require.Error(t, err)
}
