//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const (
	personalDeploymentTestCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	personalDeploymentTestDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// personalDeploymentCatalogStub 只返回测试指定的 Git/OCI 元数据，不会访问网络或本机 Docker。
type personalDeploymentCatalogStub struct {
	tags      []personalGitTag
	manifests map[string]personalRegistryManifest
	err       error
}

func (s *personalDeploymentCatalogStub) ListGitTags(context.Context, string) ([]personalGitTag, error) {
	return s.tags, s.err
}

func (s *personalDeploymentCatalogStub) ResolveRegistryTag(_ context.Context, _ string, tag string) (personalRegistryManifest, error) {
	if s.err != nil {
		return personalRegistryManifest{}, s.err
	}
	manifest, ok := s.manifests[tag]
	if !ok {
		return personalRegistryManifest{}, registryHTTPError{StatusCode: 404}
	}
	return manifest, nil
}

// newPersonalDeploymentTestService 创建固定用户 Git/镜像仓库配置，所有外部查询均由 stub 接管。
func newPersonalDeploymentTestService(catalog personalDeploymentCatalog) *PersonalImageDeploymentService {
	return &PersonalImageDeploymentService{
		部署配置: config.PersonalDeploymentConfig{
			GitRepository: "example/sub2api",
			RegistryImage: "ghcr.io/example/sub2api:latest",
			MaxVersions:   3,
		},
		版本目录: catalog,
	}
}

func TestPersonalDeploymentListVersionsRequiresMatchingGitCommitAndOCIRevision(t *testing.T) {
	service := newPersonalDeploymentTestService(&personalDeploymentCatalogStub{
		tags: []personalGitTag{
			{Name: "v0.1.161-custom.1", Commit: personalDeploymentTestCommit},
			{Name: "v0.1.162-custom.1", Commit: personalDeploymentTestCommit},
		},
		manifests: map[string]personalRegistryManifest{
			"v0.1.161-custom.1": {Digest: personalDeploymentTestDigest, Revision: personalDeploymentTestCommit},
			"v0.1.162-custom.1": {Digest: personalDeploymentTestDigest, Revision: personalDeploymentTestCommit},
		},
	})

	versions, err := service.ListVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, "v0.1.162-custom.1", versions[0].Tag)
	require.Equal(t, "ghcr.io/example/sub2api@"+personalDeploymentTestDigest, versions[0].Reference)
}

func TestPersonalDeploymentListVersionsRejectsMismatchedOCIRevision(t *testing.T) {
	service := newPersonalDeploymentTestService(&personalDeploymentCatalogStub{
		tags: []personalGitTag{{Name: "v0.1.162-custom.1", Commit: personalDeploymentTestCommit}},
		manifests: map[string]personalRegistryManifest{
			"v0.1.162-custom.1": {Digest: personalDeploymentTestDigest, Revision: "cccccccccccccccccccccccccccccccccccccccc"},
		},
	})

	_, err := service.ListVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "Git tag 与镜像 OCI revision 不一致")
}

func TestPersonalDeploymentListVersionsReturnsChineseRedactedRegistryFailure(t *testing.T) {
	service := newPersonalDeploymentTestService(&personalDeploymentCatalogStub{
		tags: []personalGitTag{{Name: "v0.1.162-custom.1", Commit: personalDeploymentTestCommit}},
		err:  errors.New("authorization=secret-value registry unavailable"),
	})

	_, err := service.ListVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "读取你的 Git 仓库版本标签失败")
	require.Contains(t, err.Error(), "authorization=***")
	require.NotContains(t, err.Error(), "secret-value")
}

func TestPersonalDeploymentConfigurationRequiresConfiguredUserRepositories(t *testing.T) {
	service := &PersonalImageDeploymentService{部署配置: config.PersonalDeploymentConfig{}}

	_, err := service.ListVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "PERSONAL_DEPLOYMENT_GIT_REPOSITORY")
}

func TestPersonalDeploymentRejectsNonDigestHelperReference(t *testing.T) {
	require.False(t, isDigestReference("ghcr.io/example/sub2api:latest"))
	require.True(t, isDigestReference("ghcr.io/example/sub2api@"+personalDeploymentTestDigest))
}
