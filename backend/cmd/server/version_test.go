package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEmbeddedVersionIsValidReleaseVersion 验证本地源码构建默认采用合法的发布版本号。
func TestEmbeddedVersionIsValidReleaseVersion(t *testing.T) {
	version := strings.TrimSpace(embeddedVersion)
	require.Regexp(t, `^\d+\.\d+\.\d+$`, version)
	require.Equal(t, version, Version)
}
