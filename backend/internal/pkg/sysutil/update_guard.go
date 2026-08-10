package sysutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const pendingUpdateSuffix = ".update-pending.json"

type pendingUpdateState struct {
	SchemaVersion int       `json:"schema_version"`
	TargetVersion string    `json:"target_version"`
	CreatedAt     time.Time `json:"created_at"`
	BootAttempted bool      `json:"boot_attempted"`
}

func PendingUpdatePath(executablePath string) string {
	return executablePath + pendingUpdateSuffix
}

// ArmPendingUpdate records the rollback intent only after the new executable
// has replaced the old one and its .backup is present.
func ArmPendingUpdate(executablePath, targetVersion string) error {
	if _, err := os.Stat(executablePath + ".backup"); err != nil {
		return fmt.Errorf("update backup is unavailable: %w", err)
	}
	state := pendingUpdateState{
		SchemaVersion: 1,
		TargetVersion: targetVersion,
		CreatedAt:     time.Now().UTC(),
	}
	return writePendingUpdateState(PendingUpdatePath(executablePath), state)
}

// PreparePendingUpdateBoot is called once during a normal server startup. The
// first boot marks the new binary as attempted. Reaching a second boot before
// ConfirmPendingUpdate means the previous boot failed, so the backup is
// restored and the caller must exit for the supervisor to start it.
func PreparePendingUpdateBoot(executablePath string) (restored bool, err error) {
	statePath := PendingUpdatePath(executablePath)
	data, err := os.ReadFile(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read pending update state: %w", err)
	}

	var state pendingUpdateState
	if err := json.Unmarshal(data, &state); err != nil || state.SchemaVersion != 1 {
		return false, fmt.Errorf("invalid pending update state")
	}
	if !state.BootAttempted {
		state.BootAttempted = true
		if err := writePendingUpdateState(statePath, state); err != nil {
			return false, err
		}
		return false, nil
	}

	if runtime.GOOS == "windows" {
		return false, fmt.Errorf("automatic executable rollback is unsupported on Windows")
	}
	backupPath := executablePath + ".backup"
	if _, err := os.Stat(backupPath); err != nil {
		return false, fmt.Errorf("pending update failed but backup is unavailable: %w", err)
	}
	if err := os.Rename(backupPath, executablePath); err != nil {
		return false, fmt.Errorf("restore update backup: %w", err)
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove pending update state after restore: %w", err)
	}
	return true, nil
}

func ConfirmPendingUpdate(executablePath string) error {
	err := os.Remove(PendingUpdatePath(executablePath))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func ClearPendingUpdate(executablePath string) error {
	return ConfirmPendingUpdate(executablePath)
}

func HasPendingUpdate(executablePath string) (bool, error) {
	_, err := os.Stat(PendingUpdatePath(executablePath))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func writePendingUpdateState(path string, state pendingUpdateState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".sub2api-update-state-*")
	if err != nil {
		return fmt.Errorf("create pending update state: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("commit pending update state: %w", err)
	}
	return nil
}
