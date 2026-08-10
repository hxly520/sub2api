package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateUpdateConfigAcceptsPrivateReleaseSettings(t *testing.T) {
	require.NoError(t, validateUpdateConfig(UpdateConfig{
		Repository:  "hxly520/sub2api",
		DockerImage: "ghcr.io/hxly520/sub2api",
		Channel:     "stable",
	}))
}

func TestValidateUpdateConfigRejectsCommandBearingValues(t *testing.T) {
	tests := []UpdateConfig{
		{Repository: "hxly520/sub2api\ninvalid"},
		{Repository: "../sub2api"},
		{DockerImage: "ghcr.io/hxly520/sub2api;whoami"},
		{DockerImage: "$(whoami)"},
		{Channel: "stable && whoami"},
	}
	for _, config := range tests {
		require.Error(t, validateUpdateConfig(config), "%+v", config)
	}
}
