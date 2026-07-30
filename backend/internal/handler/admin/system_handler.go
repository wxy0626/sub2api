package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/sysutil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SystemHandler handles system-related operations
type SystemHandler struct {
	updateSvc             systemUpdateService
	personalDeploymentSvc systemPersonalDeploymentService
	lockSvc               *service.SystemOperationLockService
}

// systemUpdateTimeout bounds a full in-place update or rollback: the release
// manifest fetch plus a large binary download over slow links. It must stay
// above the GitHub download client timeout (10 minutes) so the download owns
// its own deadline.
const systemUpdateTimeout = 15 * time.Minute

// systemUpdateContext detaches a long-running update/rollback from the HTTP
// request lifetime. Browsers and reverse proxies commonly abort idle requests
// after 30-60s (axios default, nginx proxy_read_timeout), which canceled
// c.Request.Context() mid-download and killed the update with
// "download failed: context canceled" (#4504). The swap keeps running after a
// client disconnect; a later retry then hits the system operation lock or
// reports "Already up to date".
func systemUpdateContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, systemUpdateTimeout)
}

type systemUpdateService interface {
	CheckUpdate(ctx context.Context, force bool) (*service.UpdateInfo, error)
	PerformUpdate(ctx context.Context) error
}

// systemPersonalDeploymentService 是用户自有 Git tag 与 OCI digest 驱动的受限部署接口。
type systemPersonalDeploymentService interface {
	ListVersions(ctx context.Context) ([]service.PersonalDeploymentVersion, error)
	ScheduleDeployment(ctx context.Context, tag, digest string) error
	ScheduleLatestDeployment(ctx context.Context) (service.PersonalDeploymentVersion, error)
}

// NewSystemHandler creates a new SystemHandler
func NewSystemHandler(updateSvc systemUpdateService, personalDeploymentSvc systemPersonalDeploymentService, lockSvc *service.SystemOperationLockService) *SystemHandler {
	return &SystemHandler{
		updateSvc:             updateSvc,
		personalDeploymentSvc: personalDeploymentSvc,
		lockSvc:               lockSvc,
	}
}

// GetVersion returns the current version
// GET /api/v1/admin/system/version
func (h *SystemHandler) GetVersion(c *gin.Context) {
	info, _ := h.updateSvc.CheckUpdate(c.Request.Context(), false)
	response.Success(c, gin.H{
		"version": info.CurrentVersion,
	})
}

// CheckUpdates checks for available updates
// GET /api/v1/admin/system/check-updates
func (h *SystemHandler) CheckUpdates(c *gin.Context) {
	force := c.Query("force") == "true"
	info, err := h.updateSvc.CheckUpdate(c.Request.Context(), force)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, info)
}

// PerformUpdate 明确阻止上游 release 二进制覆盖用户自有镜像部署。
// POST /api/v1/admin/system/update
func (h *SystemHandler) PerformUpdate(c *gin.Context) {
	response.ErrorFrom(c, service.PersonalDeploymentRequestError("上游 Release 仅用于版本提示，当前服务不会下载或安装上游二进制。请在“我的可部署版本”中选择已由你的 Git tag 和镜像 digest 验证的版本。技术详情：upstream binary update is disabled for personal image deployments"))
}

// GetPersonalDeploymentVersions 列出用户 Git tag 与 OCI digest/revision 已验证的可部署版本。
// GET /api/v1/admin/system/deployment-versions
func (h *SystemHandler) GetPersonalDeploymentVersions(c *gin.Context) {
	if h.personalDeploymentSvc == nil {
		response.ErrorFrom(c, service.PersonalDeploymentUnavailableError())
		return
	}
	versions, err := h.personalDeploymentSvc.ListVersions(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"versions": versions,
	})
}

// DeployLatestPersonalVersion 从用户自有发布链路中部署最新已验证 Git tag 对应的镜像。
// POST /api/v1/admin/system/deployment/update
func (h *SystemHandler) DeployLatestPersonalVersion(c *gin.Context) {
	if h.personalDeploymentSvc == nil {
		response.ErrorFrom(c, service.PersonalDeploymentUnavailableError())
		return
	}
	operationID := buildSystemOperationID(c, "deployment:update-latest")
	payload := gin.H{"operation_id": operationID}
	executeAdminIdempotentJSON(c, "admin.system.deployment.update", payload, service.DefaultSystemOperationIdempotencyTTL(), func(ctx context.Context) (any, error) {
		lock, release, err := h.acquireSystemLock(ctx, operationID)
		if err != nil {
			return nil, err
		}
		var releaseReason string
		succeeded := false
		defer func() { release(releaseReason, succeeded) }()

		deployCtx, cancel := systemUpdateContext(ctx)
		defer cancel()
		version, err := h.personalDeploymentSvc.ScheduleLatestDeployment(deployCtx)
		if err != nil {
			releaseReason = "PERSONAL_DEPLOYMENT_UPDATE_FAILED"
			return nil, err
		}
		succeeded = true
		return gin.H{
			"message":           "已安排部署你的最新 Git 镜像版本，Sub2API 服务将自动替换并恢复。PostgreSQL、Redis、数据卷、账号配置和源码工作区不会被重建或清空。",
			"need_restart":      false,
			"restart_scheduled": true,
			"tag":               version.Tag,
			"digest":            version.Digest,
			"operation_id":      lock.OperationID(),
		}, nil
	})
}

// Rollback 将当前应用替换为用户自有 Git tag 对应的已验证 OCI digest 镜像。
// The helper only replaces the current sub2api container and preserves its mounts.
// POST /api/v1/admin/system/rollback
func (h *SystemHandler) Rollback(c *gin.Context) {
	var req struct {
		Tag    string `json:"tag"`
		Digest string `json:"digest"`
	}
	if c.Request.Body == nil || c.Request.ContentLength <= 0 {
		response.ErrorFrom(c, service.PersonalDeploymentRequestError("请求缺少 tag 和 digest。请先从“我的可部署版本”列表选择一个版本。技术详情：request body is empty"))
		return
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, service.PersonalDeploymentRequestError("回退请求格式无效。请刷新页面后重新选择你的发布镜像版本。技术详情："+err.Error()))
		return
	}
	targetTag := strings.TrimSpace(req.Tag)
	targetDigest := strings.TrimSpace(req.Digest)
	if targetTag == "" || targetDigest == "" {
		response.ErrorFrom(c, service.PersonalDeploymentRequestError("请求缺少 tag 或 digest。请先从“我的可部署版本”列表选择一个版本。技术详情：tag or digest is empty"))
		return
	}
	if h.personalDeploymentSvc == nil {
		response.ErrorFrom(c, service.PersonalDeploymentUnavailableError())
		return
	}

	operation := "rollback:personal-image:" + targetTag + ":" + targetDigest
	operationID := buildSystemOperationID(c, operation)
	payload := gin.H{"operation_id": operationID, "tag": targetTag, "digest": targetDigest}
	executeAdminIdempotentJSON(c, "admin.system.rollback", payload, service.DefaultSystemOperationIdempotencyTTL(), func(ctx context.Context) (any, error) {
		lock, release, err := h.acquireSystemLock(ctx, operationID)
		if err != nil {
			return nil, err
		}
		var releaseReason string
		succeeded := false
		defer func() {
			release(releaseReason, succeeded)
		}()

		// 先拉取并核验自有 digest，再由 helper 替换当前服务；HTTP 请求可在替换前完整返回。
		rollbackCtx, cancel := systemUpdateContext(ctx)
		defer cancel()
		err = h.personalDeploymentSvc.ScheduleDeployment(rollbackCtx, targetTag, targetDigest)
		if err != nil {
			releaseReason = "SYSTEM_ROLLBACK_FAILED"
			return nil, err
		}
		succeeded = true

		return gin.H{
			"message":           "已安排使用你的 Git tag 对应镜像回退，Sub2API 服务将自动替换并恢复。PostgreSQL、Redis、数据卷、账号配置和源码工作区不会被重建或清空。",
			"need_restart":      false,
			"restart_scheduled": true,
			"tag":               targetTag,
			"digest":            targetDigest,
			"operation_id":      lock.OperationID(),
		}, nil
	})
}

// RestartService restarts the systemd service
// POST /api/v1/admin/system/restart
func (h *SystemHandler) RestartService(c *gin.Context) {
	operationID := buildSystemOperationID(c, "restart")
	payload := gin.H{"operation_id": operationID}
	executeAdminIdempotentJSON(c, "admin.system.restart", payload, service.DefaultSystemOperationIdempotencyTTL(), func(ctx context.Context) (any, error) {
		lock, release, err := h.acquireSystemLock(ctx, operationID)
		if err != nil {
			return nil, err
		}
		succeeded := false
		defer func() {
			release("", succeeded)
		}()

		// Schedule service restart in background after sending response
		// This ensures the client receives the success response before the service restarts
		go func() {
			// Wait a moment to ensure the response is sent
			time.Sleep(500 * time.Millisecond)
			sysutil.RestartServiceAsync()
		}()
		succeeded = true
		return gin.H{
			"message":      "Service restart initiated",
			"operation_id": lock.OperationID(),
		}, nil
	})
}

func (h *SystemHandler) acquireSystemLock(
	ctx context.Context,
	operationID string,
) (*service.SystemOperationLock, func(string, bool), error) {
	if h.lockSvc == nil {
		return nil, nil, service.ErrIdempotencyStoreUnavail
	}
	lock, err := h.lockSvc.Acquire(ctx, operationID)
	if err != nil {
		return nil, nil, err
	}
	release := func(reason string, succeeded bool) {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = h.lockSvc.Release(releaseCtx, lock, succeeded, reason)
	}
	return lock, release, nil
}

func buildSystemOperationID(c *gin.Context, operation string) string {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		return "sysop-" + operation + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	actorScope := "admin:0"
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		actorScope = "admin:" + strconv.FormatInt(subject.UserID, 10)
	}
	seed := operation + "|" + actorScope + "|" + c.FullPath() + "|" + key
	hash := service.HashIdempotencyKey(seed)
	if len(hash) > 24 {
		hash = hash[:24]
	}
	return "sysop-" + hash
}
