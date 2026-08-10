package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/sysutil"
)

const updateHealthConfirmationTimeout = 90 * time.Second

func currentExecutablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func preparePendingUpdateBoot() bool {
	executable, err := currentExecutablePath()
	if err != nil {
		log.Printf("Update rollback guard could not resolve executable: %v", err)
		return false
	}
	restored, err := sysutil.PreparePendingUpdateBoot(executable)
	if err != nil {
		log.Printf("Update rollback guard failed: %v", err)
		return false
	}
	if restored {
		log.Println("The previous update did not become healthy; restored the backup binary")
		log.Println("Exiting so the process supervisor can start the restored version")
		return true
	}
	return false
}

func confirmPendingUpdateWhenHealthy(server *http.Server) {
	if server == nil {
		return
	}
	executable, err := currentExecutablePath()
	if err != nil {
		log.Printf("Update health confirmation could not resolve executable: %v", err)
		return
	}
	pending, err := sysutil.HasPendingUpdate(executable)
	if err != nil || !pending {
		if err != nil {
			log.Printf("Update health confirmation could not read state: %v", err)
		}
		return
	}

	healthURL, err := localHealthURL(server.Addr)
	if err != nil {
		log.Printf("Update health confirmation could not build health URL: %v", err)
		return
	}
	go func() {
		transport := &http.Transport{Proxy: nil}
		defer transport.CloseIdleConnections()
		client := &http.Client{Timeout: 2 * time.Second, Transport: transport}
		deadline := time.Now().Add(updateHealthConfirmationTimeout)
		for time.Now().Before(deadline) {
			resp, requestErr := client.Get(healthURL)
			if requestErr == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					if confirmErr := sysutil.ConfirmPendingUpdate(executable); confirmErr != nil {
						log.Printf("Update became healthy but confirmation failed: %v", confirmErr)
						return
					}
					log.Println("Updated version passed health confirmation")
					return
				}
			}
			time.Sleep(time.Second)
		}
		log.Printf("Updated version did not pass /health within %s; restarting for automatic rollback", updateHealthConfirmationTimeout)
		sysutil.RestartServiceAsync()
	}()
}

func localHealthURL(address string) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", fmt.Errorf("empty server address")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/health", nil
}
