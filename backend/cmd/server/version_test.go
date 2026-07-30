package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEmbeddedVersionIsValidReleaseVersion 验证本地源码构建默认采用合法的版本号。
func TestEmbeddedVersionIsValidReleaseVersion(t *testing.T) {
	version := strings.TrimSpace(embeddedVersion)
	require.Regexp(t, `^\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.-]+)?$`, version)
	require.Equal(t, version, Version)
}
