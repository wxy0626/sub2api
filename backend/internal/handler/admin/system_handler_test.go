//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type systemHandlerUpdateServiceStub struct {
	performErr         error
	updateInfo         *service.UpdateInfo
	checkErr           error
	checkForces        []bool
	performCall        int
	performCtxErr      error
	performHasDeadline bool
}

func (s *systemHandlerUpdateServiceStub) CheckUpdate(_ context.Context, force bool) (*service.UpdateInfo, error) {
	s.checkForces = append(s.checkForces, force)
	return s.updateInfo, s.checkErr
}

func (s *systemHandlerUpdateServiceStub) PerformUpdate(ctx context.Context) error {
	s.performCall++
	s.performCtxErr = ctx.Err()
	_, s.performHasDeadline = ctx.Deadline()
	return s.performErr
}

type systemHandlerPersonalDeploymentStub struct {
	versions         []service.PersonalDeploymentVersion
	listErr          error
	listCalls        int
	scheduledTags    []string
	scheduledDigests []string
	scheduleErr      error
	latestVersion    service.PersonalDeploymentVersion
	latestErr        error
	latestCalls      int
}

func (s *systemHandlerPersonalDeploymentStub) ListVersions(context.Context) ([]service.PersonalDeploymentVersion, error) {
	s.listCalls++
	return s.versions, s.listErr
}

func (s *systemHandlerPersonalDeploymentStub) ScheduleDeployment(_ context.Context, tag, digest string) error {
	s.scheduledTags = append(s.scheduledTags, tag)
	s.scheduledDigests = append(s.scheduledDigests, digest)
	return s.scheduleErr
}

func (s *systemHandlerPersonalDeploymentStub) ScheduleLatestDeployment(context.Context) (service.PersonalDeploymentVersion, error) {
	s.latestCalls++
	return s.latestVersion, s.latestErr
}

type systemUpdateResponseEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Message          string `json:"message"`
		AlreadyUpToDate  bool   `json:"already_up_to_date"`
		RestartScheduled bool   `json:"restart_scheduled"`
		Tag              string `json:"tag"`
		Digest           string `json:"digest"`
		CurrentVersion   string `json:"current_version"`
		LatestVersion    string `json:"latest_version"`
		OperationID      string `json:"operation_id"`
	} `json:"data"`
}

type systemUpdateErrorEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newSystemHandlerTestRouter(t *testing.T, updateSvc *systemHandlerUpdateServiceStub, personalDeploymentSvc *systemHandlerPersonalDeploymentStub, repo *memoryIdempotencyRepoStub) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() {
		service.SetDefaultIdempotencyCoordinator(nil)
	})

	lockSvc := service.NewSystemOperationLockService(repo, service.IdempotencyConfig{
		ProcessingTimeout:  time.Second,
		SystemOperationTTL: time.Minute,
	})
	handler := NewSystemHandler(updateSvc, personalDeploymentSvc, lockSvc)

	router := gin.New()
	router.GET("/api/v1/admin/system/version", handler.GetVersion)
	router.POST("/api/v1/admin/system/update", handler.PerformUpdate)
	router.POST("/api/v1/admin/system/deployment/update", handler.DeployLatestPersonalVersion)
	router.POST("/api/v1/admin/system/rollback", handler.Rollback)
	router.GET("/api/v1/admin/system/deployment-versions", handler.GetPersonalDeploymentVersions)
	return router
}

// TestSystemHandlerGetVersionReturnsCurrentBuildVersion 验证管理端版本接口原样返回后端构建版本。
func TestSystemHandlerGetVersionReturnsCurrentBuildVersion(t *testing.T) {
	// buildVersion 模拟由后端构建阶段注入并传入更新服务的版本号。
	const buildVersion = "build-version"
	updateSvc := &systemHandlerUpdateServiceStub{
		updateInfo: &service.UpdateInfo{CurrentVersion: buildVersion},
	}
	router := newSystemHandlerTestRouter(t, updateSvc, &systemHandlerPersonalDeploymentStub{}, newMemoryIdempotencyRepoStub())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/version", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var response struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, buildVersion, response.Data.Version)
	require.Equal(t, []bool{false}, updateSvc.checkForces)
}

func requireSystemLockStatus(t *testing.T, repo *memoryIdempotencyRepoStub, wantStatus string) {
	t.Helper()
	repo.mu.Lock()
	defer repo.mu.Unlock()

	for _, record := range repo.data {
		if record.Status == wantStatus {
			return
		}
	}
	t.Fatalf("system lock status %q not found in records: %#v", wantStatus, repo.data)
}

func TestSystemHandlerPerformUpdateRejectsUpstreamBinaryReplacement(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		performErr: service.ErrNoUpdateAvailable,
		updateInfo: &service.UpdateInfo{
			CurrentVersion: "0.1.132",
			LatestVersion:  "0.1.132",
			HasUpdate:      false,
		},
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, &systemHandlerPersonalDeploymentStub{}, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update", nil)
	req.Header.Set("Idempotency-Key", "already-up-to-date")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, updateSvc.performCall)
	require.Empty(t, updateSvc.checkForces)
	require.Contains(t, rec.Body.String(), "上游 Release 仅用于版本提示")
}

func TestSystemHandlerPerformUpdateFailureStillReturnsInternalError(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{
		performErr: errors.New("download failed"),
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, &systemHandlerPersonalDeploymentStub{}, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update", nil)
	req.Header.Set("Idempotency-Key", "real-failure")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 0, updateSvc.performCall)
}

// TestSystemHandlerPerformUpdateSurvivesClientDisconnect reproduces #4504:
// the browser or a reverse proxy (axios 30s default, nginx proxy_read_timeout
// 60s) aborts the long-running update request and cancels the request
// context. The download must keep running on a detached, bounded context
// instead of dying with "download failed: context canceled".
func TestSystemHandlerPerformUpdateSurvivesClientDisconnect(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, &systemHandlerPersonalDeploymentStub{}, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update", nil)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	req = req.WithContext(canceledCtx)
	req.Header.Set("Idempotency-Key", "disconnected-update")
	router.ServeHTTP(rec, req)

	require.Equal(t, 0, updateSvc.performCall)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSystemHandlerRollbackSchedulesAllowlistedPersonalImage(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	personalDeploymentSvc := &systemHandlerPersonalDeploymentStub{}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, personalDeploymentSvc, repo)
	tag := "v0.1.162-custom.1"
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/rollback",
		strings.NewReader(`{"tag":"`+tag+`","digest":"`+digest+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "personal-image-rollback")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{tag}, personalDeploymentSvc.scheduledTags)
	require.Equal(t, []string{digest}, personalDeploymentSvc.scheduledDigests)
	requireSystemLockStatus(t, repo, service.IdempotencyStatusSucceeded)

	var body systemUpdateResponseEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.True(t, body.Data.RestartScheduled)
	require.Equal(t, tag, body.Data.Tag)
	require.Equal(t, digest, body.Data.Digest)
	require.Contains(t, body.Data.Message, "PostgreSQL")
}

func TestSystemHandlerDeployLatestPersonalVersionUsesServiceLatestSelection(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	personalDeploymentSvc := &systemHandlerPersonalDeploymentStub{
		latestVersion: service.PersonalDeploymentVersion{
			Tag: "v0.1.163-custom.1", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, personalDeploymentSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/deployment/update", nil)
	req.Header.Set("Idempotency-Key", "deploy-latest-personal")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, personalDeploymentSvc.latestCalls)
	requireSystemLockStatus(t, repo, service.IdempotencyStatusSucceeded)
	require.Contains(t, rec.Body.String(), "v0.1.163-custom.1")
	require.Contains(t, rec.Body.String(), "部署你的最新 Git 镜像版本")
}

func TestSystemHandlerRollbackRejectsMissingImageID(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	personalDeploymentSvc := &systemHandlerPersonalDeploymentStub{}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, personalDeploymentSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/rollback", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, personalDeploymentSvc.scheduledTags)
	require.Contains(t, rec.Body.String(), "tag 和 digest")
}

func TestSystemHandlerGetPersonalDeploymentVersions(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	personalDeploymentSvc := &systemHandlerPersonalDeploymentStub{versions: []service.PersonalDeploymentVersion{
		{Tag: "v0.1.162-custom.1", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Reference: "ghcr.io/example/sub2api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, personalDeploymentSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/deployment-versions", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, personalDeploymentSvc.listCalls)
	require.Contains(t, rec.Body.String(), "v0.1.162-custom.1")
	require.Contains(t, rec.Body.String(), "versions")
}

func TestSystemHandlerGetPersonalDeploymentVersionsReturnsChineseErrorAndRedactsDetail(t *testing.T) {
	updateSvc := &systemHandlerUpdateServiceStub{}
	personalDeploymentSvc := &systemHandlerPersonalDeploymentStub{listErr: service.PersonalDeploymentRequestError("读取 Git tag 失败。技术详情：authorization=secret-value")}
	repo := newMemoryIdempotencyRepoStub()
	router := newSystemHandlerTestRouter(t, updateSvc, personalDeploymentSvc, repo)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/deployment-versions", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "读取 Git tag 失败")
	require.Contains(t, rec.Body.String(), "authorization=***")
	require.NotContains(t, rec.Body.String(), "secret-value")
}
