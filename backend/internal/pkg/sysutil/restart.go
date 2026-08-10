// Package sysutil provides system-level utilities for process management.
package sysutil

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"
)

// ErrAutomaticRestartUnsupported indicates that no supported Linux supervisor
// can restart the process after its graceful shutdown.
var ErrAutomaticRestartUnsupported = errors.New("automatic restart is supported only on Linux supervisors")

// RestartService triggers the application's existing signal-driven shutdown.
//
// This relies on systemd or Docker's restart policy to automatically restart
// the service after it exits:
//   - Simple and reliable
//   - No sudo permissions needed
//   - No complex process management
//   - Leverages systemd's native restart capability
//
// Prerequisites:
//   - Linux process supervised by systemd or Docker
//   - Supervisor configured with an automatic restart policy
func RestartService() error {
	return ScheduleRestart(100 * time.Millisecond)
}

// ScheduleRestart schedules the application's existing graceful shutdown path
// after delay. It returns only after the platform and current process have been
// validated, so callers can report scheduling failures before returning success.
func ScheduleRestart(delay time.Duration) error {
	if runtime.GOOS != "linux" {
		return ErrAutomaticRestartUnsupported
	}

	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		return fmt.Errorf("find current process: %w", err)
	}

	log.Println("Initiating graceful service restart...")
	log.Println("The configured process supervisor will restart the service")

	// Let the HTTP handler flush its response, then enter main's SIGINT/SIGTERM
	// shutdown path so in-flight requests and deferred cleanup are not skipped.
	go func() {
		time.Sleep(delay)
		if signalErr := process.Signal(os.Interrupt); signalErr != nil {
			log.Printf("failed to signal graceful restart: %v", signalErr)
		}
	}()

	return nil
}

// RestartServiceAsync is a fire-and-forget version of RestartService.
// It logs errors instead of returning them, suitable for goroutine usage.
func RestartServiceAsync() {
	if err := RestartService(); err != nil {
		log.Printf("Service restart failed: %v", err)
		log.Println("Please restart the service manually: sudo systemctl restart sub2api")
	}
}
