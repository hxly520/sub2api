package sysutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPendingUpdateBootConfirmsOrRestores(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("automatic executable rollback is Linux-only")
	}
	dir := t.TempDir()
	executable := filepath.Join(dir, "sub2api")
	require.NoError(t, os.WriteFile(executable, []byte("new"), 0755))
	require.NoError(t, os.WriteFile(executable+".backup", []byte("old"), 0755))
	require.NoError(t, ArmPendingUpdate(executable, "0.1.173-52t.2"))

	restored, err := PreparePendingUpdateBoot(executable)
	require.NoError(t, err)
	require.False(t, restored)

	restored, err = PreparePendingUpdateBoot(executable)
	require.NoError(t, err)
	require.True(t, restored)
	data, err := os.ReadFile(executable)
	require.NoError(t, err)
	require.Equal(t, "old", string(data))
	_, err = os.Stat(PendingUpdatePath(executable))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestConfirmPendingUpdatePreventsRestore(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "sub2api")
	require.NoError(t, os.WriteFile(executable, []byte("new"), 0755))
	require.NoError(t, os.WriteFile(executable+".backup", []byte("old"), 0755))
	require.NoError(t, ArmPendingUpdate(executable, "0.1.173-52t.2"))
	restored, err := PreparePendingUpdateBoot(executable)
	require.NoError(t, err)
	require.False(t, restored)
	require.NoError(t, ConfirmPendingUpdate(executable))

	restored, err = PreparePendingUpdateBoot(executable)
	require.NoError(t, err)
	require.False(t, restored)
}
