//go:build unit

package sysutil

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScheduleRestartRejectsUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-Linux platform guard")
	}

	require.ErrorIs(t, ScheduleRestart(time.Hour), ErrAutomaticRestartUnsupported)
}
