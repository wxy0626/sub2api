package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

const (
	personalDeploymentDockerSocketPath       = "/var/run/docker.sock"
	personalDeploymentHelperFlag             = "--personal-image-deploy-helper"
	personalDeploymentParentEnv              = "PERSONAL_DEPLOYMENT_PARENT_ID"
	personalDeploymentTargetReferenceEnv     = "PERSONAL_DEPLOYMENT_TARGET_REFERENCE"
	personalDeploymentExpectedCommitEnv      = "PERSONAL_DEPLOYMENT_EXPECTED_COMMIT"
	personalDeploymentServiceName            = "sub2api"
	personalDeploymentRevisionLabel          = "org.opencontainers.image.revision"
	personalDeploymentSourceLabel            = "org.opencontainers.image.source"
	personalDeploymentDefaultVersionLimit    = 3
	personalDeploymentRegistryRequestTimeout = 45 * time.Second
)

// personalDeploymentDigestPattern 限制管理员请求只能使用 OCI sha256 内容摘要。
var personalDeploymentDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// personalDeploymentTagPattern 限制可部署版本为发布工具创建的 vSemVer Git tag。
var personalDeploymentTagPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)

// personalDeploymentRepositoryPattern 限制 GitHub 仓库配置为 owner/repository。
var personalDeploymentRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// PersonalDeploymentVersion 是已由用户 Git tag、OCI digest 与镜像 revision 三方验证的部署版本。
type PersonalDeploymentVersion struct {
	Tag       string `json:"tag"`
	Commit    string `json:"commit"`
	Digest    string `json:"digest"`
	Reference string `json:"reference"`
}

// personalGitTag 是 GitHub Tags API 返回的最小可信版本映射。
type personalGitTag struct {
	Name   string
	Commit string
}

// personalRegistryManifest 是 OCI registry 验证后的不可变镜像元数据。
type personalRegistryManifest struct {
	Digest   string
	Revision string
}

// personalDeploymentCatalog 使 GitHub tag 与 OCI manifest 查询可被定向单元测试替换。
type personalDeploymentCatalog interface {
	ListGitTags(ctx context.Context, repository string) ([]personalGitTag, error)
	ResolveRegistryTag(ctx context.Context, registryImage, tag string) (personalRegistryManifest, error)
}

// personalDeploymentDockerEngine 声明自有镜像部署所需的最小 Docker 权限集合。
type personalDeploymentDockerEngine interface {
	ContainerInspect(context.Context, string) (container.InspectResponse, error)
	ContainerCreate(context.Context, *container.Config, *container.HostConfig, *network.NetworkingConfig, any, string) (container.CreateResponse, error)
	ContainerStart(context.Context, string, container.StartOptions) error
	ContainerStop(context.Context, string, container.StopOptions) error
	ContainerRemove(context.Context, string, container.RemoveOptions) error
	ContainerRename(context.Context, string, string) error
	ImagePull(context.Context, string, image.PullOptions) (io.ReadCloser, error)
	ImageInspectWithRaw(context.Context, string) (image.InspectResponse, []byte, error)
	Close() error
}

// personalDeploymentDockerAdapter 将 Docker SDK 封装为受限接口，避免业务层直接访问任意 Docker API。
type personalDeploymentDockerAdapter struct {
	client *client.Client
}

func (d *personalDeploymentDockerAdapter) ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error) {
	return d.client.ContainerInspect(ctx, id)
}

func (d *personalDeploymentDockerAdapter) ContainerCreate(ctx context.Context, cfg *container.Config, hostCfg *container.HostConfig, networkCfg *network.NetworkingConfig, _ any, name string) (container.CreateResponse, error) {
	return d.client.ContainerCreate(ctx, cfg, hostCfg, networkCfg, nil, name)
}

func (d *personalDeploymentDockerAdapter) ContainerStart(ctx context.Context, id string, options container.StartOptions) error {
	return d.client.ContainerStart(ctx, id, options)
}

func (d *personalDeploymentDockerAdapter) ContainerStop(ctx context.Context, id string, options container.StopOptions) error {
	return d.client.ContainerStop(ctx, id, options)
}

func (d *personalDeploymentDockerAdapter) ContainerRemove(ctx context.Context, id string, options container.RemoveOptions) error {
	return d.client.ContainerRemove(ctx, id, options)
}

func (d *personalDeploymentDockerAdapter) ContainerRename(ctx context.Context, id, name string) error {
	return d.client.ContainerRename(ctx, id, name)
}

func (d *personalDeploymentDockerAdapter) ImagePull(ctx context.Context, reference string, options image.PullOptions) (io.ReadCloser, error) {
	return d.client.ImagePull(ctx, reference, options)
}

func (d *personalDeploymentDockerAdapter) ImageInspectWithRaw(ctx context.Context, reference string) (image.InspectResponse, []byte, error) {
	return d.client.ImageInspectWithRaw(ctx, reference)
}

func (d *personalDeploymentDockerAdapter) Close() error {
	return d.client.Close()
}

// PersonalImageDeploymentService 仅允许部署用户自有 Git tag 映射到的 OCI digest 镜像。
type PersonalImageDeploymentService struct {
	// 部署配置 提供用户 Git、镜像仓库与只读凭据，所有字段只从服务端环境读取。
	部署配置 config.PersonalDeploymentConfig
	// 版本目录 客户端负责同时查询 Git tag 与 OCI manifest，避免把可变 tag 当成部署凭据。
	版本目录 personalDeploymentCatalog
	// 创建Docker引擎客户端 保持可替换依赖，供定向单元测试模拟容器替换和镜像拉取。
	创建Docker引擎客户端 func() (personalDeploymentDockerEngine, error)
	// 获取当前容器ID 使用 hostname 定位自身，不接受客户端指定的 Docker 目标。
	获取当前容器ID func() (string, error)
	// 检查DockerSocket 先给出可执行的缺失 socket 中文说明，避免泄露 Docker 环境变量。
	检查DockerSocket func() bool
}

// NewPersonalImageDeploymentService 创建用户自有 Git/GHCR 发布链路的部署服务。
func NewPersonalImageDeploymentService(cfg *config.Config) *PersonalImageDeploymentService {
	部署配置 := config.PersonalDeploymentConfig{}
	if cfg != nil {
		部署配置 = cfg.PersonalDeployment
	}
	return &PersonalImageDeploymentService{
		部署配置: 部署配置,
		版本目录: &personalDeploymentHTTPCatalog{
			httpClient:       &http.Client{Timeout: personalDeploymentRegistryRequestTimeout},
			gitHubToken:      strings.TrimSpace(部署配置.GitHubToken),
			registryUsername: strings.TrimSpace(部署配置.RegistryUsername),
			registryToken:    strings.TrimSpace(部署配置.RegistryToken),
		},
		创建Docker引擎客户端: func() (personalDeploymentDockerEngine, error) {
			apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				return nil, err
			}
			return &personalDeploymentDockerAdapter{client: apiClient}, nil
		},
		获取当前容器ID: os.Hostname,
		检查DockerSocket: func() bool {
			info, err := os.Stat(personalDeploymentDockerSocketPath)
			return err == nil && info.Mode()&os.ModeSocket != 0
		},
	}
}

// ListVersions 返回用户仓库中同时具备 Git tag、OCI digest 和 revision 标签验证的版本。
func (s *PersonalImageDeploymentService) ListVersions(ctx context.Context) ([]PersonalDeploymentVersion, error) {
	gitRepository, registryImage, limit, err := s.normalizedSettings()
	if err != nil {
		return nil, err
	}
	if s.版本目录 == nil {
		return nil, personalDeploymentInternalError("自有发布版本目录未初始化。请确认服务已重启并加载最新部署配置。", errors.New("personal deployment catalog is unavailable"))
	}

	tags, err := s.版本目录.ListGitTags(ctx, gitRepository)
	if err != nil {
		return nil, personalDeploymentInternalError("读取你的 Git 仓库版本标签失败。请检查 PERSONAL_DEPLOYMENT_GITHUB_TOKEN 是否具备 contents 读取权限，以及 PERSONAL_DEPLOYMENT_GIT_REPOSITORY 配置。", err)
	}
	sort.SliceStable(tags, func(i, j int) bool {
		return comparePersonalDeploymentTags(tags[i].Name, tags[j].Name) > 0
	})

	versions := make([]PersonalDeploymentVersion, 0, limit)
	for _, tag := range tags {
		if len(versions) >= limit {
			break
		}
		if !personalDeploymentTagPattern.MatchString(tag.Name) || !isFullCommitSHA(tag.Commit) {
			continue
		}
		manifest, err := s.版本目录.ResolveRegistryTag(ctx, registryImage, tag.Name)
		if err != nil {
			if isRegistryNotFound(err) {
				continue
			}
			return nil, personalDeploymentInternalError("读取你的镜像仓库版本元数据失败。请检查 PERSONAL_DEPLOYMENT_REGISTRY_USERNAME、PERSONAL_DEPLOYMENT_REGISTRY_TOKEN 与镜像仓库访问权限。", err)
		}
		if !personalDeploymentDigestPattern.MatchString(manifest.Digest) {
			return nil, personalDeploymentInternalError("镜像仓库返回的 digest 格式无效。请重新发布该 Git tag 对应镜像。", fmt.Errorf("invalid registry digest for tag %s", tag.Name))
		}
		if !strings.EqualFold(strings.TrimSpace(manifest.Revision), tag.Commit) {
			return nil, personalDeploymentInternalError("Git tag 与镜像 OCI revision 不一致，已拒绝列出该版本。请使用发布工具重新构建并推送镜像。", fmt.Errorf("tag=%s git_commit=%s image_revision=%s", tag.Name, tag.Commit, manifest.Revision))
		}
		versions = append(versions, PersonalDeploymentVersion{
			Tag:       tag.Name,
			Commit:    tag.Commit,
			Digest:    manifest.Digest,
			Reference: registryImage + "@" + manifest.Digest,
		})
	}
	return versions, nil
}

// ScheduleDeployment 拉取并验证不可变镜像后，调度独立 helper 仅替换 Sub2API 应用容器。
func (s *PersonalImageDeploymentService) ScheduleDeployment(ctx context.Context, tag, digest string) error {
	tag = strings.TrimSpace(tag)
	digest = strings.TrimSpace(digest)
	if !personalDeploymentTagPattern.MatchString(tag) || !personalDeploymentDigestPattern.MatchString(digest) {
		return PersonalDeploymentRequestError("请求中的 tag 或 digest 格式无效。请刷新“我的可部署版本”列表后重新选择。技术详情：tag 必须为 vSemVer，digest 必须为 sha256")
	}
	versions, err := s.ListVersions(ctx)
	if err != nil {
		return err
	}
	var target PersonalDeploymentVersion
	for _, candidate := range versions {
		if candidate.Tag == tag && candidate.Digest == digest {
			target = candidate
			break
		}
	}
	if target.Reference == "" {
		return infraerrors.BadRequest("PERSONAL_DEPLOYMENT_VERSION_NOT_ALLOWED", "所选版本不在已验证的“我的可部署版本”列表中。请刷新列表；若刚发布，请确认 Git tag、镜像 digest 和 OCI revision 均已生成。技术详情：tag/digest whitelist rejected")
	}

	engine, err := s.openEngine()
	if err != nil {
		return err
	}
	defer func() { _ = engine.Close() }()

	current, err := s.currentSub2APIContainer(ctx, engine)
	if err != nil {
		return err
	}
	if err := pullAndVerifyPersonalImage(ctx, engine, target.Reference, target.Commit, s.registryAuth()); err != nil {
		return err
	}

	// helper 只获得本次白名单确认后的容器 ID、digest 引用和 commit，不接受浏览器传入的 Docker 参数。
	helperConfig := &container.Config{
		Image: current.Config.Image,
		Cmd:   []string{"/app/sub2api", personalDeploymentHelperFlag},
		Env: []string{
			personalDeploymentParentEnv + "=" + current.ID,
			personalDeploymentTargetReferenceEnv + "=" + target.Reference,
			personalDeploymentExpectedCommitEnv + "=" + target.Commit,
		},
		Labels: map[string]string{
			"sub2api.personal_deployment_helper": "true",
		},
	}
	helperHostConfig := &container.HostConfig{
		AutoRemove:  true,
		NetworkMode: "none",
		Mounts: []mount.Mount{{
			Type:   mount.TypeBind,
			Source: personalDeploymentDockerSocketPath,
			Target: personalDeploymentDockerSocketPath,
		}},
	}
	helperName := personalDeploymentServiceName + "-deploy-helper-" + shortPersonalDockerID(current.ID)
	helper, err := engine.ContainerCreate(ctx, helperConfig, helperHostConfig, nil, nil, helperName)
	if err != nil {
		return personalDeploymentInternalError("创建自有镜像部署任务失败，当前服务没有被替换。请确认 Docker socket 已挂载且当前容器镜像可启动 helper。", err)
	}
	if err := engine.ContainerStart(ctx, helper.ID, container.StartOptions{}); err != nil {
		_ = engine.ContainerRemove(context.Background(), helper.ID, container.RemoveOptions{Force: true, RemoveVolumes: false})
		return personalDeploymentInternalError("启动自有镜像部署任务失败，当前服务没有被替换。请确认 Docker 服务正常运行后重试。", err)
	}
	return nil
}

// ScheduleLatestDeployment 只从已验证版本列表的第一项部署用户最新 Git tag，浏览器不参与选择最新 tag。
func (s *PersonalImageDeploymentService) ScheduleLatestDeployment(ctx context.Context) (PersonalDeploymentVersion, error) {
	versions, err := s.ListVersions(ctx)
	if err != nil {
		return PersonalDeploymentVersion{}, err
	}
	if len(versions) == 0 {
		return PersonalDeploymentVersion{}, infraerrors.BadRequest("PERSONAL_DEPLOYMENT_VERSION_NOT_FOUND", "未找到可部署的自有镜像版本。请先运行发布工具，确认 Git tag 已推送且 GHCR 镜像带有相同的 OCI revision。技术详情：no verified personal deployment versions")
	}
	latest := versions[0]
	if err := s.ScheduleDeployment(ctx, latest.Tag, latest.Digest); err != nil {
		return PersonalDeploymentVersion{}, err
	}
	return latest, nil
}

// RunPersonalImageDeploymentHelper 在独立 helper 容器中执行实际替换，确保 HTTP 成功响应能先返回给管理员。
func RunPersonalImageDeploymentHelper(ctx context.Context) error {
	parentID := strings.TrimSpace(os.Getenv(personalDeploymentParentEnv))
	targetReference := strings.TrimSpace(os.Getenv(personalDeploymentTargetReferenceEnv))
	expectedCommit := strings.TrimSpace(os.Getenv(personalDeploymentExpectedCommitEnv))
	if parentID == "" || !isDigestReference(targetReference) || !isFullCommitSHA(expectedCommit) {
		return errors.New("自有镜像部署任务参数无效。技术详情：helper 环境中缺少经过校验的 parent_id、digest reference 或 commit")
	}

	service := NewPersonalImageDeploymentService(nil)
	if !service.检查DockerSocket() {
		return errors.New("未检测到 Docker socket。技术详情：/var/run/docker.sock 不存在或不是 Unix socket")
	}
	engine, err := service.创建Docker引擎客户端()
	if err != nil {
		return personalDeploymentTechnicalError("连接 Docker Engine 失败。", err)
	}
	defer func() { _ = engine.Close() }()

	// 短暂等待让管理员浏览器先收到成功响应，避免请求在替换过程中中断。
	time.Sleep(1200 * time.Millisecond)
	return recreatePersonalDeploymentContainer(ctx, engine, parentID, targetReference, expectedCommit)
}

// openEngine 在连接 Docker 前验证 socket 已挂载，避免无效的宿主环境请求。
func (s *PersonalImageDeploymentService) openEngine() (personalDeploymentDockerEngine, error) {
	if !s.检查DockerSocket() {
		return nil, infraerrors.InternalServer("PERSONAL_DEPLOYMENT_DOCKER_SOCKET_MISSING", "未检测到 Docker socket，镜像部署只支持受控 Docker Compose 环境。请在部署 Compose 中挂载 /var/run/docker.sock 后重试。技术详情：/var/run/docker.sock 不存在或不可访问")
	}
	engine, err := s.创建Docker引擎客户端()
	if err != nil {
		return nil, personalDeploymentInternalError("连接 Docker Engine 失败。请确认 Docker 服务正在运行，并检查 Docker socket 挂载权限。", err)
	}
	return engine, nil
}

// currentSub2APIContainer 读取并校验当前容器属于受支持的 Compose 应用服务。
func (s *PersonalImageDeploymentService) currentSub2APIContainer(ctx context.Context, engine personalDeploymentDockerEngine) (container.InspectResponse, error) {
	containerID, err := s.获取当前容器ID()
	if err != nil {
		return container.InspectResponse{}, personalDeploymentInternalError("无法定位当前 Sub2API 容器。请确认服务由 Docker Compose 启动。", err)
	}
	current, err := engine.ContainerInspect(ctx, containerID)
	if err != nil {
		return container.InspectResponse{}, personalDeploymentInternalError("无法读取当前 Sub2API 容器。请确认 Docker 服务正常运行后重试。", err)
	}
	if err := validatePersonalDeploymentContainer(current); err != nil {
		return container.InspectResponse{}, err
	}
	return current, nil
}

// recreatePersonalDeploymentContainer 用已经拉取并验证的 digest 镜像替换应用容器，完整保留原始挂载与网络配置。
func recreatePersonalDeploymentContainer(ctx context.Context, engine personalDeploymentDockerEngine, parentID, targetReference, expectedCommit string) error {
	parent, err := engine.ContainerInspect(ctx, parentID)
	if err != nil {
		return personalDeploymentTechnicalError("无法读取待替换的 Sub2API 容器。", err)
	}
	if err := validatePersonalDeploymentContainer(parent); err != nil {
		return err
	}
	if err := verifyPulledPersonalImage(ctx, engine, targetReference, expectedCommit); err != nil {
		return err
	}

	originalName := strings.TrimPrefix(parent.Name, "/")
	if originalName == "" {
		return errors.New("当前 Sub2API 容器名称无效。技术详情：Docker inspect 未返回容器名称")
	}
	temporaryName := originalName + "-deploy-candidate-" + shortPersonalDockerID(targetReference)
	targetConfig := clonePersonalContainerConfig(parent.Config)
	targetConfig.Image = targetReference
	targetConfig.Hostname = ""
	targetHostConfig := clonePersonalHostConfig(parent.HostConfig)
	targetNetworkConfig := clonePersonalNetworkingConfig(parent.NetworkSettings)
	candidate, err := engine.ContainerCreate(ctx, targetConfig, targetHostConfig, targetNetworkConfig, nil, temporaryName)
	if err != nil {
		return personalDeploymentTechnicalError("目标自有镜像无法创建替换容器，当前服务保持运行。", err)
	}

	cleanupCandidate := func() {
		_ = engine.ContainerRemove(context.Background(), candidate.ID, container.RemoveOptions{Force: true, RemoveVolumes: false})
	}
	stopTimeout := 15
	if err := engine.ContainerStop(ctx, parent.ID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		cleanupCandidate()
		return personalDeploymentTechnicalError("停止当前 Sub2API 容器失败，当前服务未被删除。", err)
	}
	if err := engine.ContainerRemove(ctx, parent.ID, container.RemoveOptions{Force: false, RemoveVolumes: false}); err != nil {
		cleanupCandidate()
		return personalDeploymentTechnicalError("删除旧 Sub2API 容器失败，数据卷和数据库均未删除。", err)
	}
	if err := engine.ContainerRename(ctx, candidate.ID, originalName); err != nil {
		return personalDeploymentTechnicalError("已停止旧容器，但替换容器重命名失败。请使用已验证的 digest 镜像重新创建 Sub2API 服务。", err)
	}
	if err := engine.ContainerStart(ctx, candidate.ID, container.StartOptions{}); err != nil {
		if recoveryErr := restorePersonalDeploymentContainer(ctx, engine, candidate.ID, originalName, parent); recoveryErr != nil {
			return personalDeploymentTechnicalError("目标镜像启动失败，且自动恢复原镜像失败。请使用回退前的 digest 镜像重建 Sub2API 容器。", fmt.Errorf("start=%v; restore=%v", err, recoveryErr))
		}
		return personalDeploymentTechnicalError("目标镜像启动失败，系统已自动恢复到替换前的镜像。", err)
	}
	return nil
}

// pullAndVerifyPersonalImage 在停止当前服务前拉取 digest 引用并验证镜像 revision，失败时当前服务保持运行。
func pullAndVerifyPersonalImage(ctx context.Context, engine personalDeploymentDockerEngine, targetReference, expectedCommit, registryAuth string) error {
	stream, err := engine.ImagePull(ctx, targetReference, image.PullOptions{RegistryAuth: registryAuth})
	if err != nil {
		return personalDeploymentInternalError("拉取你的镜像仓库版本失败，当前服务保持运行。请检查镜像仓库登录权限和 digest 是否仍存在。", err)
	}
	defer func() { _ = stream.Close() }()
	if err := consumePersonalDockerPullStream(stream); err != nil {
		return personalDeploymentInternalError("拉取你的镜像仓库版本失败，当前服务保持运行。请检查 GHCR packages:read 权限与镜像仓库配置。", err)
	}
	return verifyPulledPersonalImage(ctx, engine, targetReference, expectedCommit)
}

// verifyPulledPersonalImage 再次校验本机已拉取镜像的 OCI revision，防止 registry tag 被替换后误部署。
func verifyPulledPersonalImage(ctx context.Context, engine personalDeploymentDockerEngine, targetReference, expectedCommit string) error {
	inspected, _, err := engine.ImageInspectWithRaw(ctx, targetReference)
	if err != nil {
		return personalDeploymentTechnicalError("读取已拉取镜像元数据失败。", err)
	}
	if inspected.Config == nil || inspected.Config.Labels == nil {
		return errors.New("已拉取镜像缺少 OCI revision 标签，已拒绝替换当前服务。技术详情：org.opencontainers.image.revision is missing")
	}
	if !strings.EqualFold(strings.TrimSpace(inspected.Config.Labels[personalDeploymentRevisionLabel]), expectedCommit) {
		return fmt.Errorf("已拉取镜像 OCI revision 与 Git tag 不一致，已拒绝替换当前服务。技术详情：expected_commit=%s image_revision=%s", expectedCommit, inspected.Config.Labels[personalDeploymentRevisionLabel])
	}
	if !containsPersonalDigest(inspected.RepoDigests, targetReference) {
		return fmt.Errorf("已拉取镜像 digest 与白名单不一致，已拒绝替换当前服务。技术详情：expected_reference=%s", targetReference)
	}
	return nil
}

// consumePersonalDockerPullStream 读取 Docker pull 流并将 JSON error 转为可脱敏的错误，不向管理员界面暴露原始流。
func consumePersonalDockerPullStream(stream io.Reader) error {
	decoder := json.NewDecoder(stream)
	for {
		var event struct {
			Error string `json:"error"`
		}
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if strings.TrimSpace(event.Error) != "" {
			return errors.New(event.Error)
		}
	}
}

// restorePersonalDeploymentContainer 在候选镜像启动失败后使用原配置恢复应用容器。
func restorePersonalDeploymentContainer(ctx context.Context, engine personalDeploymentDockerEngine, candidateID, originalName string, parent container.InspectResponse) error {
	_ = engine.ContainerRemove(ctx, candidateID, container.RemoveOptions{Force: true, RemoveVolumes: false})
	recoveryConfig := clonePersonalContainerConfig(parent.Config)
	recoveryConfig.Hostname = ""
	recovery, err := engine.ContainerCreate(ctx, recoveryConfig, clonePersonalHostConfig(parent.HostConfig), clonePersonalNetworkingConfig(parent.NetworkSettings), nil, originalName)
	if err != nil {
		return err
	}
	return engine.ContainerStart(ctx, recovery.ID, container.StartOptions{})
}

// validatePersonalDeploymentContainer 限制操作目标为当前 Compose 的 sub2api 服务，不依赖本机镜像命名。
func validatePersonalDeploymentContainer(current container.InspectResponse) error {
	if current.Config == nil || current.Config.Labels == nil || current.Config.Labels["com.docker.compose.service"] != personalDeploymentServiceName {
		return infraerrors.BadRequest("PERSONAL_DEPLOYMENT_CONTAINER_NOT_ALLOWED", "当前容器不是受支持的 Sub2API Docker Compose 服务，已拒绝替换操作。请使用 Compose 启动名为 sub2api 的应用容器。技术详情：com.docker.compose.service 必须为 sub2api")
	}
	return nil
}

// clonePersonalContainerConfig 复制容器配置，避免候选容器与原始 inspect 数据共享可变对象。
func clonePersonalContainerConfig(source *container.Config) *container.Config {
	if source == nil {
		return &container.Config{}
	}
	copyConfig := *source
	copyConfig.Env = append([]string(nil), source.Env...)
	copyConfig.Cmd = append([]string(nil), source.Cmd...)
	copyConfig.Entrypoint = append([]string(nil), source.Entrypoint...)
	copyConfig.Labels = clonePersonalStringMap(source.Labels)
	copyConfig.Volumes = clonePersonalEmptyStructMap(source.Volumes)
	return &copyConfig
}

// clonePersonalHostConfig 复制宿主机配置与挂载声明，确保端口、数据库目录和数据卷不会漂移。
func clonePersonalHostConfig(source *container.HostConfig) *container.HostConfig {
	if source == nil {
		return &container.HostConfig{}
	}
	copyConfig := *source
	copyConfig.Binds = append([]string(nil), source.Binds...)
	copyConfig.Mounts = append([]mount.Mount(nil), source.Mounts...)
	copyConfig.DNS = append([]string(nil), source.DNS...)
	copyConfig.DNSOptions = append([]string(nil), source.DNSOptions...)
	copyConfig.DNSSearch = append([]string(nil), source.DNSSearch...)
	copyConfig.ExtraHosts = append([]string(nil), source.ExtraHosts...)
	copyConfig.Links = append([]string(nil), source.Links...)
	copyConfig.SecurityOpt = append([]string(nil), source.SecurityOpt...)
	copyConfig.PortBindings = source.PortBindings
	return &copyConfig
}

// clonePersonalNetworkingConfig 提取 Docker 接受的网络配置字段，跳过运行时分配的地址。
func clonePersonalNetworkingConfig(source *container.NetworkSettings) *network.NetworkingConfig {
	if source == nil || len(source.Networks) == 0 {
		return nil
	}
	endpoints := make(map[string]*network.EndpointSettings, len(source.Networks))
	for name, endpoint := range source.Networks {
		if endpoint == nil {
			continue
		}
		endpointCopy := &network.EndpointSettings{
			Links:      append([]string(nil), endpoint.Links...),
			Aliases:    append([]string(nil), endpoint.Aliases...),
			MacAddress: endpoint.MacAddress,
			DriverOpts: clonePersonalStringMap(endpoint.DriverOpts),
			GwPriority: endpoint.GwPriority,
		}
		if endpoint.IPAMConfig != nil {
			endpointCopy.IPAMConfig = endpoint.IPAMConfig.Copy()
		}
		endpoints[name] = endpointCopy
	}
	return &network.NetworkingConfig{EndpointsConfig: endpoints}
}

// clonePersonalStringMap 复制字符串映射，避免候选配置修改原容器 inspect 数据。
func clonePersonalStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// clonePersonalEmptyStructMap 复制 Docker 使用的集合型 map。
func clonePersonalEmptyStructMap(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(source))
	for key := range source {
		result[key] = struct{}{}
	}
	return result
}

// shortPersonalDockerID 生成仅用于 helper 和候选容器名称的短标识。
func shortPersonalDockerID(value string) string {
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

// normalizedSettings 校验并规范化部署来源，避免配置中的可变 tag 进入 digest 白名单流程。
func (s *PersonalImageDeploymentService) normalizedSettings() (string, string, int, error) {
	gitRepository := strings.TrimSpace(s.部署配置.GitRepository)
	if !personalDeploymentRepositoryPattern.MatchString(gitRepository) {
		return "", "", 0, PersonalDeploymentConfigurationError("未配置有效的 PERSONAL_DEPLOYMENT_GIT_REPOSITORY。请设置为你的 GitHub 仓库 owner/repository，例如 example/sub2api。技术详情：git_repository must match owner/repository")
	}
	registryImage, err := normalizePersonalRegistryImage(s.部署配置.RegistryImage)
	if err != nil {
		return "", "", 0, PersonalDeploymentConfigurationError("未配置有效的 PERSONAL_DEPLOYMENT_REGISTRY_IMAGE。请设置为完整镜像仓库，例如 ghcr.io/example/sub2api，不能使用仅含 tag 的本机镜像名。技术详情：" + err.Error())
	}
	limit := s.部署配置.MaxVersions
	if limit <= 0 {
		limit = personalDeploymentDefaultVersionLimit
	}
	if limit > 20 {
		limit = 20
	}
	return gitRepository, registryImage, limit, nil
}

// registryAuth 按 Docker Engine 要求编码只读 registry 凭据；凭据仅传给 Docker API，不进入错误文本。
func (s *PersonalImageDeploymentService) registryAuth() string {
	username := strings.TrimSpace(s.部署配置.RegistryUsername)
	token := strings.TrimSpace(s.部署配置.RegistryToken)
	if username == "" || token == "" {
		return ""
	}
	auth := registry.AuthConfig{Username: username, Password: token}
	payload, err := json.Marshal(auth)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(payload)
}

// PersonalDeploymentConfigurationError 返回带中文处理方式且已脱敏的配置错误。
func PersonalDeploymentConfigurationError(detail string) error {
	return infraerrors.BadRequest("PERSONAL_DEPLOYMENT_CONFIGURATION_INVALID", logredact.RedactText(detail, "authorization", "cookie", "api_key", "apikey", "token", "secret", "password"))
}

// PersonalDeploymentRequestError 返回带中文处理方式且已脱敏的管理员请求错误。
func PersonalDeploymentRequestError(detail string) error {
	return infraerrors.BadRequest("PERSONAL_DEPLOYMENT_REQUEST_INVALID", logredact.RedactText(detail, "authorization", "cookie", "api_key", "apikey", "token", "secret", "password"))
}

// PersonalDeploymentUnavailableError 返回服务未正确注入时的中文诊断。
func PersonalDeploymentUnavailableError() error {
	return infraerrors.InternalServer("PERSONAL_DEPLOYMENT_SERVICE_UNAVAILABLE", "自有 Git 镜像部署服务未初始化。请确认服务已使用包含部署模块的镜像重建，并在 Compose 中挂载 Docker socket。技术详情：personal deployment service is unavailable")
}

// personalDeploymentInternalError 组合中文处理建议和经脱敏的技术详情。
func personalDeploymentInternalError(summary string, err error) error {
	// 不附加原始 cause，避免错误链被日志或测试直接格式化时泄露 registry/Git 凭据。
	return infraerrors.InternalServer("PERSONAL_DEPLOYMENT_FAILED", summary+"技术详情："+personalDeploymentTechnicalDetail(err))
}

// personalDeploymentTechnicalError 用于 helper 内部失败，保留经脱敏的技术详情供容器日志诊断。
func personalDeploymentTechnicalError(summary string, err error) error {
	return fmt.Errorf("%s技术详情：%s", summary, personalDeploymentTechnicalDetail(err))
}

// personalDeploymentTechnicalDetail 统一脱敏 GitHub、OCI registry 与 Docker 返回的凭据类字段。
func personalDeploymentTechnicalDetail(err error) string {
	if err == nil {
		return ""
	}
	return logredact.RedactText(err.Error(), "authorization", "cookie", "api_key", "apikey", "token", "secret", "password")
}

// personalDeploymentHTTPCatalog 同时读取用户 GitHub tag 与 OCI registry manifest，不依赖本机镜像列表。
type personalDeploymentHTTPCatalog struct {
	httpClient       *http.Client
	gitHubToken      string
	registryUsername string
	registryToken    string
}

// ListGitTags 读取用户仓库发布 tag；只读 token 仅发送到 api.github.com。
func (c *personalDeploymentHTTPCatalog) ListGitTags(ctx context.Context, repository string) ([]personalGitTag, error) {
	requestURL := "https://api.github.com/repos/" + repository + "/tags?per_page=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Sub2API-Personal-Deployment")
	if c.gitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.gitHubToken)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var payload []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	tags := make([]personalGitTag, 0, len(payload))
	for _, item := range payload {
		tags = append(tags, personalGitTag{Name: strings.TrimSpace(item.Name), Commit: strings.TrimSpace(item.Commit.SHA)})
	}
	return tags, nil
}

// ResolveRegistryTag 读取 OCI manifest digest 与 config revision 标签，确认 tag 对应不可变部署物。
func (c *personalDeploymentHTTPCatalog) ResolveRegistryTag(ctx context.Context, registryImage, tag string) (personalRegistryManifest, error) {
	host, repository, err := splitPersonalRegistryImage(registryImage)
	if err != nil {
		return personalRegistryManifest{}, err
	}
	manifest, digest, err := c.getRegistryManifest(ctx, host, repository, tag)
	if err != nil {
		return personalRegistryManifest{}, err
	}
	if len(manifest.Manifests) > 0 {
		descriptor, ok := selectPersonalPlatformManifest(manifest.Manifests)
		if !ok {
			return personalRegistryManifest{}, fmt.Errorf("registry manifest does not contain %s/%s", runtime.GOOS, runtime.GOARCH)
		}
		manifest, _, err = c.getRegistryManifest(ctx, host, repository, descriptor.Digest)
		if err != nil {
			return personalRegistryManifest{}, err
		}
	}
	if !personalDeploymentDigestPattern.MatchString(manifest.Config.Digest) {
		return personalRegistryManifest{}, errors.New("registry manifest config digest is missing")
	}
	configBlob, err := c.getRegistryBlob(ctx, host, repository, manifest.Config.Digest)
	if err != nil {
		return personalRegistryManifest{}, err
	}
	var configPayload struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(configBlob, &configPayload); err != nil {
		return personalRegistryManifest{}, err
	}
	return personalRegistryManifest{
		Digest:   digest,
		Revision: strings.TrimSpace(configPayload.Config.Labels[personalDeploymentRevisionLabel]),
	}, nil
}

// registryManifestPayload 是 OCI/Docker manifest 和 manifest list 共同需要的字段集合。
type registryManifestPayload struct {
	Config struct {
		Digest string `json:"digest"`
	} `json:"config"`
	Manifests []struct {
		Digest   string `json:"digest"`
		Platform struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
		} `json:"platform"`
	} `json:"manifests"`
}

// getRegistryManifest 通过 registry v2 API 获取 manifest，并保留服务端返回的 Docker-Content-Digest。
func (c *personalDeploymentHTTPCatalog) getRegistryManifest(ctx context.Context, host, repository, reference string) (registryManifestPayload, string, error) {
	path := "/v2/" + repository + "/manifests/" + reference
	body, headers, err := c.registryGet(ctx, host, repository, path, "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
	if err != nil {
		return registryManifestPayload{}, "", err
	}
	var manifest registryManifestPayload
	if err := json.Unmarshal(body, &manifest); err != nil {
		return registryManifestPayload{}, "", err
	}
	digest := strings.TrimSpace(headers.Get("Docker-Content-Digest"))
	if !personalDeploymentDigestPattern.MatchString(digest) {
		return registryManifestPayload{}, "", errors.New("registry response did not include a valid Docker-Content-Digest")
	}
	return manifest, digest, nil
}

// getRegistryBlob 读取 OCI config blob，以便校验发布工具写入的 revision 标签。
func (c *personalDeploymentHTTPCatalog) getRegistryBlob(ctx context.Context, host, repository, digest string) ([]byte, error) {
	body, _, err := c.registryGet(ctx, host, repository, "/v2/"+repository+"/blobs/"+digest, "application/vnd.oci.image.config.v1+json, application/vnd.docker.container.image.v1+json")
	return body, err
}

// registryGet 处理 OCI Bearer challenge；凭据只发送到配置的 registry host 或其 challenge realm。
func (c *personalDeploymentHTTPCatalog) registryGet(ctx context.Context, host, repository, requestPath, accept string) ([]byte, http.Header, error) {
	requestURL := "https://" + host + requestPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "Sub2API-Personal-Deployment")
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		defer func() { _ = resp.Body.Close() }()
		return c.readRegistryResponse(resp)
	}
	challenge := resp.Header.Get("WWW-Authenticate")
	_ = resp.Body.Close()
	bearerToken, err := c.getRegistryBearerToken(ctx, challenge, repository)
	if err != nil {
		return nil, nil, err
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "Sub2API-Personal-Deployment")
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	resp, err = c.client().Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return c.readRegistryResponse(resp)
}

// readRegistryResponse 将 registry 状态码转换为不包含凭据的技术错误。
func (c *personalDeploymentHTTPCatalog) readRegistryResponse(resp *http.Response) ([]byte, http.Header, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, nil, registryHTTPError{StatusCode: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, nil, err
	}
	return body, resp.Header.Clone(), nil
}

// getRegistryBearerToken 依据 Docker Registry v2 Bearer challenge 请求只读 token。
func (c *personalDeploymentHTTPCatalog) getRegistryBearerToken(ctx context.Context, challenge, repository string) (string, error) {
	realm, serviceName, err := parsePersonalBearerChallenge(challenge)
	if err != nil {
		return "", err
	}
	realmURL, err := url.Parse(realm)
	if err != nil || realmURL.Scheme != "https" || realmURL.User != nil {
		return "", errors.New("registry bearer challenge realm is invalid")
	}
	query := realmURL.Query()
	query.Set("service", serviceName)
	query.Set("scope", "repository:"+repository+":pull")
	realmURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realmURL.String(), nil)
	if err != nil {
		return "", err
	}
	if c.registryUsername != "" && c.registryToken != "" {
		req.SetBasicAuth(c.registryUsername, c.registryToken)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry token endpoint returned %d", resp.StatusCode)
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Token) != "" {
		return strings.TrimSpace(payload.Token), nil
	}
	if strings.TrimSpace(payload.AccessToken) != "" {
		return strings.TrimSpace(payload.AccessToken), nil
	}
	return "", errors.New("registry token endpoint returned an empty token")
}

// client 返回可替换 HTTP 客户端，避免测试或启动异常时 nil 解引用。
func (c *personalDeploymentHTTPCatalog) client() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}
	return &http.Client{Timeout: personalDeploymentRegistryRequestTimeout}
}

// registryHTTPError 保留 registry HTTP 状态，用于忽略尚未发布镜像的 Git tag。
type registryHTTPError struct {
	StatusCode int
}

func (e registryHTTPError) Error() string {
	return "registry returned " + strconv.Itoa(e.StatusCode)
}

// isRegistryNotFound 仅忽略 tag 尚无镜像的 404，其他访问失败必须向管理员显示。
func isRegistryNotFound(err error) bool {
	var httpErr registryHTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

// parsePersonalBearerChallenge 解析 Docker Registry Bearer header，拒绝非 Bearer challenge。
func parsePersonalBearerChallenge(challenge string) (string, string, error) {
	challenge = strings.TrimSpace(challenge)
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", "", errors.New("registry did not return a Bearer authentication challenge")
	}
	params := make(map[string]string)
	for _, part := range strings.Split(strings.TrimSpace(challenge[len("Bearer "):]), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		params[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	realm := strings.TrimSpace(params["realm"])
	serviceName := strings.TrimSpace(params["service"])
	if realm == "" || serviceName == "" {
		return "", "", errors.New("registry Bearer challenge is missing realm or service")
	}
	return realm, serviceName, nil
}

// selectPersonalPlatformManifest 选择运行当前管理服务的 Docker 平台对应的子 manifest。
func selectPersonalPlatformManifest(manifests []struct {
	Digest   string `json:"digest"`
	Platform struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
	} `json:"platform"`
}) (struct {
	Digest   string `json:"digest"`
	Platform struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
	} `json:"platform"`
}, bool) {
	for _, descriptor := range manifests {
		if descriptor.Platform.OS == runtime.GOOS && descriptor.Platform.Architecture == runtime.GOARCH && personalDeploymentDigestPattern.MatchString(descriptor.Digest) {
			return descriptor, true
		}
	}
	return struct {
		Digest   string `json:"digest"`
		Platform struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
		} `json:"platform"`
	}{}, false
}

// normalizePersonalRegistryImage 接受 registry/repository[:tag]，并剥离可变 tag 以强制后续按 digest 部署。
func normalizePersonalRegistryImage(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" || strings.Contains(reference, "://") || strings.Contains(reference, "@") || strings.ContainsAny(reference, " \t\r\n") {
		return "", errors.New("registry_image must be a registry/repository reference")
	}
	parts := strings.Split(reference, "/")
	if len(parts) < 2 || !strings.Contains(parts[0], ".") {
		return "", errors.New("registry_image must include a registry host")
	}
	last := parts[len(parts)-1]
	if separator := strings.LastIndex(last, ":"); separator > 0 {
		last = last[:separator]
		parts[len(parts)-1] = last
	}
	if last == "" || strings.Contains(reference, "//") {
		return "", errors.New("registry_image repository is invalid")
	}
	return strings.Join(parts, "/"), nil
}

// splitPersonalRegistryImage 拆分规范化镜像仓库为 registry host 和 repository path。
func splitPersonalRegistryImage(reference string) (string, string, error) {
	normalized, err := normalizePersonalRegistryImage(reference)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(normalized, "/", 2)
	return parts[0], parts[1], nil
}

// isFullCommitSHA 只接受 GitHub Tags API 返回的完整 commit SHA。
func isFullCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

// isDigestReference 验证 helper 只能接收带 sha256 digest 的 registry 引用。
func isDigestReference(reference string) bool {
	registryImage, digest, ok := strings.Cut(reference, "@")
	_, err := normalizePersonalRegistryImage(registryImage)
	return ok && err == nil && personalDeploymentDigestPattern.MatchString(digest)
}

// containsPersonalDigest 验证 Docker inspect 的 RepoDigests 保留了服务端白名单中的精确 digest 引用。
func containsPersonalDigest(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// comparePersonalDeploymentTags 按 vSemVer 的主次补丁和预发布语义排序，无法解析的 tag 放到末尾。
func comparePersonalDeploymentTags(left, right string) int {
	parse := func(value string) (int, int, int, string, bool) {
		if !personalDeploymentTagPattern.MatchString(value) {
			return 0, 0, 0, "", false
		}
		version := strings.TrimPrefix(value, "v")
		mainVersion, suffix, _ := strings.Cut(version, "-")
		mainVersion, _, _ = strings.Cut(mainVersion, "+")
		parts := strings.Split(mainVersion, ".")
		major, _ := strconv.Atoi(parts[0])
		minor, _ := strconv.Atoi(parts[1])
		patch, _ := strconv.Atoi(parts[2])
		return major, minor, patch, suffix, true
	}
	lMajor, lMinor, lPatch, lSuffix, lOK := parse(left)
	rMajor, rMinor, rPatch, rSuffix, rOK := parse(right)
	if lOK != rOK {
		if lOK {
			return 1
		}
		return -1
	}
	for _, pair := range [][2]int{{lMajor, rMajor}, {lMinor, rMinor}, {lPatch, rPatch}} {
		if pair[0] > pair[1] {
			return 1
		}
		if pair[0] < pair[1] {
			return -1
		}
	}
	if lSuffix == "" && rSuffix != "" {
		return 1
	}
	if lSuffix != "" && rSuffix == "" {
		return -1
	}
	return strings.Compare(lSuffix, rSuffix)
}
