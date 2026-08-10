package service

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/sysutil"
	"golang.org/x/mod/semver"
)

var (
	ErrNoUpdateAvailable         = infraerrors.Conflict("ALREADY_UP_TO_DATE", "no update available; current version is latest")
	ErrRollbackVersionNotAllowed = infraerrors.BadRequest("ROLLBACK_VERSION_NOT_ALLOWED", "version is not in the allowed rollback list")
	ErrInPlaceUpdateDisabled     = infraerrors.Conflict("IN_PLACE_UPDATE_DISABLED", "in-place update is disabled for this deployment")
	ErrImageUpdateRequired       = infraerrors.Conflict("IMAGE_UPDATE_REQUIRED", "this release requires a Docker Compose image update")
	privateReleaseVersionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-52t\.([1-9][0-9]*)$`)
	fullCommitPattern            = regexp.MustCompile(`(?i)^[0-9a-f]{40}$`)
	embeddedCommitPattern        = regexp.MustCompile(`(?i)commit:\s*([0-9a-f]{40})\b`)
)

const (
	updateCacheTTL = 1200 // 20 minutes

	defaultUpdateRepository  = "hxly520/sub2api"
	defaultUpdateDockerImage = "ghcr.io/hxly520/sub2api"
	defaultUpdateChannel     = "stable"
	updateManifestAssetName  = "update-manifest.json"

	UpdatePolicySafe             = "hot-update-safe"
	UpdatePolicyImageRecommended = "image-update-recommended"
	UpdatePolicyImageRequired    = "image-update-required"

	// Security: allowed download domains for updates
	allowedDownloadHost = "github.com"
	allowedAssetHost    = "objects.githubusercontent.com"
	allowedAPIHost      = "api.github.com"
	allowedReleaseHost  = "release-assets.githubusercontent.com"

	// Security: max download size (500MB)
	maxDownloadSize = 500 * 1024 * 1024

	// Rollback: expose at most the 3 most recent versions older than current
	maxRollbackVersions = 3
	// Fetch a few extra releases so filtering (current/newer/prerelease) still leaves enough candidates
	rollbackFetchPageSize = 15
)

// UpdateCache defines cache operations for update service
type UpdateCache interface {
	GetUpdateInfo(ctx context.Context, namespace ...string) (string, error)
	SetUpdateInfo(ctx context.Context, data string, ttl time.Duration, namespace ...string) error
}

// GitHubReleaseClient 获取 GitHub release 信息的接口
type GitHubReleaseClient interface {
	FetchLatestRelease(ctx context.Context, repo string) (*GitHubRelease, error)
	FetchRecentReleases(ctx context.Context, repo string, perPage int) ([]*GitHubRelease, error)
	DownloadFile(ctx context.Context, url, dest string, maxSize int64) error
	FetchChecksumFile(ctx context.Context, url string) ([]byte, error)
}

// UpdateService handles software updates
type UpdateService struct {
	cache          UpdateCache
	githubClient   GitHubReleaseClient
	currentVersion string
	buildType      string // "source" for manual builds, "release" for CI builds
	options        UpdateOptions
}

// UpdateOptions describes the private release channel and its trust requirements.
type UpdateOptions struct {
	Repository      string
	DockerImage     string
	Channel         string
	InPlaceEnabled  bool
	RequireChecksum bool
	RequireManifest bool
}

func DefaultUpdateOptions() UpdateOptions {
	return UpdateOptions{
		Repository:      defaultUpdateRepository,
		DockerImage:     defaultUpdateDockerImage,
		Channel:         defaultUpdateChannel,
		InPlaceEnabled:  true,
		RequireChecksum: true,
		RequireManifest: true,
	}
}

// NewUpdateService creates a new UpdateService
func NewUpdateService(cache UpdateCache, githubClient GitHubReleaseClient, version, buildType string, configured ...UpdateOptions) *UpdateService {
	options := DefaultUpdateOptions()
	if len(configured) > 0 {
		options = normalizeUpdateOptions(configured[0])
	}
	return &UpdateService{
		cache:          cache,
		githubClient:   githubClient,
		currentVersion: version,
		buildType:      buildType,
		options:        options,
	}
}

func normalizeUpdateOptions(options UpdateOptions) UpdateOptions {
	options.Repository = strings.TrimSpace(options.Repository)
	options.DockerImage = strings.TrimSpace(options.DockerImage)
	options.Channel = strings.TrimSpace(options.Channel)
	if options.Repository == "" {
		options.Repository = defaultUpdateRepository
	}
	if options.DockerImage == "" {
		options.DockerImage = defaultUpdateDockerImage
	}
	if options.Channel == "" {
		options.Channel = defaultUpdateChannel
	}
	return options
}

// UpdateInfo contains update information
type UpdateInfo struct {
	CurrentVersion   string       `json:"current_version"`
	LatestVersion    string       `json:"latest_version"`
	HasUpdate        bool         `json:"has_update"`
	ReleaseInfo      *ReleaseInfo `json:"release_info,omitempty"`
	Cached           bool         `json:"cached"`
	Warning          string       `json:"warning,omitempty"`
	BuildType        string       `json:"build_type"` // "source" or "release"
	Repository       string       `json:"repository"`
	DockerImage      string       `json:"docker_image"`
	Channel          string       `json:"channel"`
	HotUpdatePolicy  string       `json:"hot_update_policy"`
	HotUpdateAllowed bool         `json:"hot_update_allowed"`
	HotUpdateReasons []string     `json:"hot_update_reasons,omitempty"`
	SourceCommit     string       `json:"source_commit,omitempty"`
}

// ReleaseInfo contains GitHub release details
type ReleaseInfo struct {
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	HTMLURL     string  `json:"html_url"`
	Assets      []Asset `json:"assets,omitempty"`
}

// Asset represents a release asset
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	APIURL      string `json:"-"`
	Size        int64  `json:"size"`
}

type UpdateManifest struct {
	SchemaVersion int      `json:"schema_version"`
	Version       string   `json:"version"`
	SourceCommit  string   `json:"source_commit"`
	Policy        string   `json:"policy"`
	Reasons       []string `json:"reasons"`
}

// GitHubRelease represents GitHub API response
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	PublishedAt string        `json:"published_at"`
	HTMLURL     string        `json:"html_url"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []GitHubAsset `json:"assets"`
}

// RollbackVersion describes a release version the system can roll back to
type RollbackVersion struct {
	Version          string   `json:"version"` // without "v" prefix, e.g. "0.1.173-52t.1"
	PublishedAt      string   `json:"published_at"`
	HTMLURL          string   `json:"html_url"`
	HotUpdatePolicy  string   `json:"hot_update_policy"`
	HotUpdateAllowed bool     `json:"hot_update_allowed"`
	HotUpdateReasons []string `json:"hot_update_reasons,omitempty"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	APIURL             string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// CheckUpdate checks for available updates
func (s *UpdateService) CheckUpdate(ctx context.Context, force bool) (*UpdateInfo, error) {
	// Try cache first
	if !force {
		if cached, err := s.getFromCache(ctx); err == nil && cached != nil {
			return cached, nil
		}
	}

	// Fetch from GitHub
	info, err := s.fetchLatestRelease(ctx)
	if err != nil {
		// A forced check is part of an update/rollback decision and must fail
		// closed. Cached assets must never be installed after the source changed
		// or the private GitHub request failed.
		if force {
			return nil, err
		}
		// Cached release metadata is display-only and scoped to this source.
		if cached, cacheErr := s.getFromCache(ctx); cacheErr == nil && cached != nil {
			cached.Warning = "Using cached data: " + err.Error()
			return cached, nil
		}
		return &UpdateInfo{
			CurrentVersion:   s.currentVersion,
			LatestVersion:    s.currentVersion,
			HasUpdate:        false,
			Warning:          err.Error(),
			BuildType:        s.buildType,
			Repository:       s.options.Repository,
			DockerImage:      s.options.DockerImage,
			Channel:          s.options.Channel,
			HotUpdatePolicy:  UpdatePolicyImageRequired,
			HotUpdateAllowed: false,
		}, nil
	}

	// Cache result
	s.saveToCache(ctx, info)
	return info, nil
}

// PerformUpdate downloads and applies the update
// Uses atomic file replacement pattern for safe in-place updates
func (s *UpdateService) PerformUpdate(ctx context.Context) error {
	if !s.options.InPlaceEnabled || s.buildType != "release" {
		return ErrInPlaceUpdateDisabled
	}
	info, err := s.CheckUpdate(ctx, true)
	if err != nil {
		return err
	}

	if !info.HasUpdate {
		return ErrNoUpdateAvailable
	}
	if !info.HotUpdateAllowed || info.HotUpdatePolicy == UpdatePolicyImageRequired {
		return ErrImageUpdateRequired
	}

	return s.applyReleaseAssets(ctx, info.LatestVersion, info.SourceCommit, info.ReleaseInfo.Assets)
}

// applyReleaseAssets downloads the platform archive from the given release assets,
// verifies its checksum, and atomically swaps the running binary.
// Shared by PerformUpdate (latest) and RollbackToVersion (specific older version).
func (s *UpdateService) applyReleaseAssets(ctx context.Context, targetVersion, sourceCommit string, releaseAssets []Asset) error {
	// Find matching archive and checksum for current platform
	archiveName := s.getArchiveName()
	var archiveAsset *Asset
	var checksumAsset *Asset

	for i := range releaseAssets {
		asset := &releaseAssets[i]
		if strings.Contains(asset.Name, archiveName) && !strings.HasSuffix(asset.Name, ".txt") {
			archiveAsset = asset
		}
		if asset.Name == "checksums.txt" {
			checksumAsset = asset
		}
	}

	if archiveAsset == nil {
		return fmt.Errorf("no compatible release found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if s.options.RequireChecksum && checksumAsset == nil {
		return fmt.Errorf("release is missing required checksums.txt")
	}

	// SECURITY: Validate download URL is from trusted domain
	downloadURL := preferredAssetURL(*archiveAsset)
	if err := validateDownloadURL(downloadURL); err != nil {
		return fmt.Errorf("invalid download URL: %w", err)
	}
	if checksumAsset != nil {
		checksumURL := preferredAssetURL(*checksumAsset)
		if err := validateDownloadURL(checksumURL); err != nil {
			return fmt.Errorf("invalid checksum URL: %w", err)
		}
	}

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	exeDir := filepath.Dir(exePath)
	if pending, pendingErr := sysutil.HasPendingUpdate(exePath); pendingErr != nil {
		return fmt.Errorf("check pending update state: %w", pendingErr)
	} else if pending {
		return fmt.Errorf("a previous update is awaiting restart confirmation")
	}

	// Create temp directory in the SAME directory as executable
	// This ensures os.Rename is atomic (same filesystem)
	tempDir, err := os.MkdirTemp(exeDir, ".sub2api-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Download archive
	archivePath := filepath.Join(tempDir, filepath.Base(archiveAsset.Name))
	if err := s.downloadFile(ctx, downloadURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if archiveAsset.Size > 0 {
		stat, statErr := os.Stat(archivePath)
		if statErr != nil {
			return fmt.Errorf("stat downloaded archive: %w", statErr)
		}
		if stat.Size() != archiveAsset.Size {
			return fmt.Errorf("download size mismatch: expected %d bytes, got %d", archiveAsset.Size, stat.Size())
		}
	}

	// Verify checksum before extracting or replacing anything.
	if checksumAsset != nil {
		if err := s.verifyChecksum(ctx, archivePath, preferredAssetURL(*checksumAsset)); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// Extract binary from archive
	newBinaryPath := filepath.Join(tempDir, "sub2api")
	if err := s.extractBinary(archivePath, newBinaryPath); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Set executable permission before replacement
	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}
	if err := validateReplacementBinary(ctx, newBinaryPath, targetVersion, sourceCommit); err != nil {
		return fmt.Errorf("replacement binary validation failed: %w", err)
	}

	// Atomic replacement using rename pattern:
	// 1. Rename current -> backup (atomic on Unix)
	// 2. Rename new -> current (atomic on Unix, same filesystem)
	// If step 2 fails, restore backup
	backupPath := exePath + ".backup"

	// Remove old backup if exists
	_ = os.Remove(backupPath)

	// Step 1: Move current binary to backup
	if err := os.Rename(exePath, backupPath); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Step 2: Move new binary to target location (atomic, same filesystem)
	if err := os.Rename(newBinaryPath, exePath); err != nil {
		// Restore backup on failure
		if restoreErr := os.Rename(backupPath, exePath); restoreErr != nil {
			return fmt.Errorf("replace failed and restore failed: %w (restore error: %v)", err, restoreErr)
		}
		return fmt.Errorf("replace failed (restored backup): %w", err)
	}
	if err := sysutil.ArmPendingUpdate(exePath, targetVersion); err != nil {
		if restoreErr := os.Rename(backupPath, exePath); restoreErr != nil {
			return fmt.Errorf("arm rollback guard failed: %w (restore error: %v)", err, restoreErr)
		}
		return fmt.Errorf("arm rollback guard failed (restored backup): %w", err)
	}

	// Success - backup file is kept for rollback capability
	// It will be cleaned up on next successful update
	return nil
}

func preferredAssetURL(asset Asset) string {
	if strings.TrimSpace(asset.APIURL) != "" {
		return asset.APIURL
	}
	return asset.DownloadURL
}

func validateReplacementBinary(ctx context.Context, binaryPath, targetVersion, sourceCommit string) error {
	if _, err := normalizeSourceCommit(sourceCommit); err != nil {
		return err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run --version: %w", err)
	}
	return validateReplacementVersionOutput(output, targetVersion, sourceCommit)
}

func validateReplacementVersionOutput(output []byte, targetVersion, sourceCommit string) error {
	want := strings.TrimPrefix(strings.TrimSpace(targetVersion), "v")
	matched := false
	fields := strings.Fields(string(output))
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "Sub2API" && strings.TrimPrefix(fields[i+1], "v") == want {
			matched = true
			break
		}
	}
	if want != "" && !matched {
		return fmt.Errorf("version output does not identify Sub2API %q", want)
	}
	wantCommit, err := normalizeSourceCommit(sourceCommit)
	if err != nil {
		return err
	}
	commitMatch := embeddedCommitPattern.FindSubmatch(output)
	if len(commitMatch) != 2 {
		return fmt.Errorf("version output does not contain a full embedded commit")
	}
	gotCommit := strings.ToLower(string(commitMatch[1]))
	if gotCommit != wantCommit {
		return fmt.Errorf("embedded commit %q does not match manifest source commit %q", gotCommit, wantCommit)
	}
	return nil
}

func normalizeSourceCommit(sourceCommit string) (string, error) {
	commit := strings.TrimSpace(sourceCommit)
	if !fullCommitPattern.MatchString(commit) {
		return "", fmt.Errorf("manifest source_commit must be a full 40-character hexadecimal commit")
	}
	return strings.ToLower(commit), nil
}

// Rollback restores the previous version
func (s *UpdateService) Rollback() error {
	if runtime.GOOS != "linux" || !s.options.InPlaceEnabled || s.buildType != "release" {
		return ErrInPlaceUpdateDisabled
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	backupFile := exePath + ".backup"
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("no backup found")
	}

	// Replace current with backup
	if err := os.Rename(backupFile, exePath); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	if err := sysutil.ClearPendingUpdate(exePath); err != nil {
		return fmt.Errorf("clear pending update state: %w", err)
	}

	return nil
}

// ListRollbackVersions returns up to maxRollbackVersions release versions that are
// strictly older than the current version (the current version itself is excluded),
// newest first. Draft and prerelease entries are skipped.
func (s *UpdateService) ListRollbackVersions(ctx context.Context) ([]RollbackVersion, error) {
	releases, err := s.fetchRollbackCandidates(ctx)
	if err != nil {
		return nil, err
	}

	versions := make([]RollbackVersion, 0, len(releases))
	for _, r := range releases {
		assets := releaseAssets(r)
		policy, reasons, sourceCommit := s.releaseUpdatePolicy(ctx, strings.TrimPrefix(r.TagName, "v"), assets)
		versions = append(versions, RollbackVersion{
			Version:          strings.TrimPrefix(r.TagName, "v"),
			PublishedAt:      r.PublishedAt,
			HTMLURL:          r.HTMLURL,
			HotUpdatePolicy:  policy,
			HotUpdateAllowed: s.hotUpdateAllowed(policy, sourceCommit),
			HotUpdateReasons: reasons,
		})
	}
	return versions, nil
}

// RollbackToVersion downloads and installs a specific older version.
// The target must be one of the versions returned by ListRollbackVersions;
// anything else (including the current version) is rejected.
func (s *UpdateService) RollbackToVersion(ctx context.Context, version string) error {
	if !s.options.InPlaceEnabled || s.buildType != "release" {
		return ErrInPlaceUpdateDisabled
	}
	target := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if target == "" {
		return ErrRollbackVersionNotAllowed
	}

	releases, err := s.fetchRollbackCandidates(ctx)
	if err != nil {
		return err
	}

	var match *GitHubRelease
	for _, r := range releases {
		if strings.TrimPrefix(r.TagName, "v") == target {
			match = r
			break
		}
	}
	if match == nil {
		return ErrRollbackVersionNotAllowed
	}

	assets := releaseAssets(match)
	policy, _, sourceCommit := s.releaseUpdatePolicy(ctx, target, assets)
	if !s.hotUpdateAllowed(policy, sourceCommit) {
		return ErrImageUpdateRequired
	}

	return s.applyReleaseAssets(ctx, target, sourceCommit, assets)
}

func releaseAssets(release *GitHubRelease) []Asset {
	if release == nil {
		return nil
	}
	assets := make([]Asset, len(release.Assets))
	for i, asset := range release.Assets {
		assets[i] = Asset{
			Name:        asset.Name,
			APIURL:      asset.APIURL,
			DownloadURL: asset.BrowserDownloadURL,
			Size:        asset.Size,
		}
	}
	return assets
}

// fetchRollbackCandidates fetches recent releases and keeps the newest
// maxRollbackVersions entries strictly older than the current version.
func (s *UpdateService) fetchRollbackCandidates(ctx context.Context) ([]*GitHubRelease, error) {
	releases, err := s.githubClient.FetchRecentReleases(ctx, s.options.Repository, rollbackFetchPageSize)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(releases))
	candidates := make([]*GitHubRelease, 0, maxRollbackVersions)
	for _, r := range releases {
		if r == nil || r.Draft || r.Prerelease {
			continue
		}
		v, versionErr := normalizePrivateReleaseVersion(r.TagName)
		if versionErr != nil || seen[v] {
			continue
		}
		// Only versions strictly older than current (also excludes current itself)
		if compareVersions(v, s.currentVersion) >= 0 {
			continue
		}
		seen[v] = true
		candidates = append(candidates, r)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return compareVersions(
			strings.TrimPrefix(candidates[i].TagName, "v"),
			strings.TrimPrefix(candidates[j].TagName, "v"),
		) > 0
	})

	if len(candidates) > maxRollbackVersions {
		candidates = candidates[:maxRollbackVersions]
	}
	return candidates, nil
}

func (s *UpdateService) fetchLatestRelease(ctx context.Context) (*UpdateInfo, error) {
	release, err := s.githubClient.FetchLatestRelease(ctx, s.options.Repository)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, fmt.Errorf("private release response was empty")
	}
	if release.Draft || release.Prerelease {
		return nil, fmt.Errorf("latest private release is not a stable published release")
	}
	latestVersion, err := normalizePrivateReleaseVersion(release.TagName)
	if err != nil {
		return nil, err
	}

	assets := releaseAssets(release)

	policy, reasons, sourceCommit := s.releaseUpdatePolicy(ctx, latestVersion, assets)

	return &UpdateInfo{
		CurrentVersion: s.currentVersion,
		LatestVersion:  latestVersion,
		HasUpdate:      compareVersions(s.currentVersion, latestVersion) < 0,
		ReleaseInfo: &ReleaseInfo{
			Name:        release.Name,
			Body:        release.Body,
			PublishedAt: release.PublishedAt,
			HTMLURL:     release.HTMLURL,
			Assets:      assets,
		},
		Cached:           false,
		BuildType:        s.buildType,
		Repository:       s.options.Repository,
		DockerImage:      s.options.DockerImage,
		Channel:          s.options.Channel,
		HotUpdatePolicy:  policy,
		HotUpdateAllowed: s.hotUpdateAllowed(policy, sourceCommit),
		HotUpdateReasons: reasons,
		SourceCommit:     sourceCommit,
	}, nil
}

func normalizePrivateReleaseVersion(raw string) (string, error) {
	version := strings.TrimSpace(raw)
	if !privateReleaseVersionPattern.MatchString(version) {
		return "", fmt.Errorf("release tag %q is not a private vX.Y.Z-52t.N version", raw)
	}
	canonical := canonicalSemver(version)
	if canonical == "" {
		return "", fmt.Errorf("release tag %q is not valid semantic versioning", raw)
	}
	return strings.TrimPrefix(canonical, "v"), nil
}

func (s *UpdateService) releaseUpdatePolicy(ctx context.Context, releaseVersion string, assets []Asset) (string, []string, string) {
	manifest, err := s.fetchUpdateManifest(ctx, releaseVersion, assets)
	if err == nil {
		return manifest.Policy, append([]string(nil), manifest.Reasons...), strings.ToLower(manifest.SourceCommit)
	}
	return UpdatePolicyImageRequired, []string{"valid update policy manifest and source commit required: " + err.Error()}, ""
}

func (s *UpdateService) hotUpdateAllowed(policy, sourceCommit string) bool {
	_, commitErr := normalizeSourceCommit(sourceCommit)
	return commitErr == nil && runtime.GOOS == "linux" && s.options.InPlaceEnabled && s.buildType == "release" && policy != UpdatePolicyImageRequired
}

func (s *UpdateService) fetchUpdateManifest(ctx context.Context, releaseVersion string, assets []Asset) (*UpdateManifest, error) {
	var manifestAsset *Asset
	for i := range assets {
		if assets[i].Name == updateManifestAssetName {
			manifestAsset = &assets[i]
			break
		}
	}
	if manifestAsset == nil {
		return nil, fmt.Errorf("%s not found", updateManifestAssetName)
	}
	manifestURL := preferredAssetURL(*manifestAsset)
	if err := validateDownloadURL(manifestURL); err != nil {
		return nil, fmt.Errorf("invalid manifest URL: %w", err)
	}
	data, err := s.githubClient.FetchChecksumFile(ctx, manifestURL)
	if err != nil {
		return nil, fmt.Errorf("download update manifest: %w", err)
	}
	var manifest UpdateManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode update manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported update manifest schema %d", manifest.SchemaVersion)
	}
	if _, err := normalizeSourceCommit(manifest.SourceCommit); err != nil {
		return nil, err
	}
	if compareVersions(manifest.Version, releaseVersion) != 0 || strings.TrimPrefix(manifest.Version, "v") != strings.TrimPrefix(releaseVersion, "v") {
		return nil, fmt.Errorf("manifest version %q does not match release %q", manifest.Version, releaseVersion)
	}
	switch manifest.Policy {
	case UpdatePolicySafe, UpdatePolicyImageRecommended, UpdatePolicyImageRequired:
	default:
		return nil, fmt.Errorf("unsupported update policy %q", manifest.Policy)
	}
	return &manifest, nil
}

func (s *UpdateService) downloadFile(ctx context.Context, downloadURL, dest string) error {
	return s.githubClient.DownloadFile(ctx, downloadURL, dest, maxDownloadSize)
}

func (s *UpdateService) getArchiveName() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	return fmt.Sprintf("%s_%s", osName, arch)
}

// validateDownloadURL checks if the URL is from an allowed domain
// SECURITY: This prevents SSRF and ensures downloads only come from trusted GitHub domains
func validateDownloadURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Must be HTTPS
	if !strings.EqualFold(parsedURL.Scheme, "https") {
		return fmt.Errorf("only HTTPS URLs are allowed")
	}
	if parsedURL.User != nil || parsedURL.Port() != "" {
		return fmt.Errorf("download URL must not include userinfo or an explicit port")
	}

	// Check against allowed hosts
	host := strings.ToLower(parsedURL.Hostname())
	if host != allowedDownloadHost && host != allowedAssetHost && host != allowedAPIHost && host != allowedReleaseHost {
		return fmt.Errorf("download from untrusted host: %s", host)
	}

	return nil
}

func (s *UpdateService) verifyChecksum(ctx context.Context, filePath, checksumURL string) error {
	// Download checksums file
	checksumData, err := s.githubClient.FetchChecksumFile(ctx, checksumURL)
	if err != nil {
		return fmt.Errorf("failed to download checksums: %w", err)
	}

	// Calculate file hash
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actualHash := hex.EncodeToString(h.Sum(nil))

	// Find expected hash in checksums file
	fileName := filepath.Base(filePath)
	scanner := bufio.NewScanner(strings.NewReader(string(checksumData)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 2 && strings.TrimPrefix(parts[1], "*") == fileName {
			if strings.EqualFold(parts[0], actualHash) {
				return nil
			}
			return fmt.Errorf("checksum mismatch: expected %s, got %s", parts[0], actualHash)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}

	return fmt.Errorf("checksum not found for %s", fileName)
}

func (s *UpdateService) extractBinary(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var reader io.Reader = f

	// Handle gzip compression
	if strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer func() { _ = gzr.Close() }()
		reader = gzr
	}

	// Handle tar archive
	if strings.Contains(archivePath, ".tar") {
		tr := tar.NewReader(reader)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}

			// SECURITY: Prevent Zip Slip / Path Traversal attack
			// Only allow files with safe base names, no directory traversal
			baseName := filepath.Base(hdr.Name)

			// Check for path traversal attempts
			if strings.Contains(hdr.Name, "..") {
				return fmt.Errorf("path traversal attempt detected: %s", hdr.Name)
			}

			// Validate the entry is a regular file
			if hdr.Typeflag != tar.TypeReg {
				continue // Skip directories and special files
			}

			// Only extract the specific binary we need
			if baseName == "sub2api" || baseName == "sub2api.exe" {
				// Additional security: limit file size (max 500MB)
				const maxBinarySize = 500 * 1024 * 1024
				if hdr.Size > maxBinarySize {
					return fmt.Errorf("binary too large: %d bytes (max %d)", hdr.Size, maxBinarySize)
				}

				out, err := os.Create(destPath)
				if err != nil {
					return err
				}

				// Use LimitReader to prevent decompression bombs
				limited := io.LimitReader(tr, maxBinarySize)
				if _, err := io.Copy(out, limited); err != nil {
					_ = out.Close()
					return err
				}
				if err := out.Close(); err != nil {
					return err
				}
				return nil
			}
		}
		return fmt.Errorf("binary not found in archive")
	}

	// Direct copy for non-tar files (with size limit)
	const maxBinarySize = 500 * 1024 * 1024
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}

	limited := io.LimitReader(reader, maxBinarySize)
	if _, err := io.Copy(out, limited); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (s *UpdateService) getFromCache(ctx context.Context) (*UpdateInfo, error) {
	data, err := s.cache.GetUpdateInfo(ctx, s.cacheNamespace())
	if err != nil {
		return nil, err
	}

	var cached struct {
		Repository       string       `json:"repository"`
		DockerImage      string       `json:"docker_image"`
		Channel          string       `json:"channel"`
		Latest           string       `json:"latest"`
		ReleaseInfo      *ReleaseInfo `json:"release_info"`
		HotUpdatePolicy  string       `json:"hot_update_policy"`
		HotUpdateAllowed bool         `json:"hot_update_allowed"`
		HotUpdateReasons []string     `json:"hot_update_reasons"`
		SourceCommit     string       `json:"source_commit"`
		Timestamp        int64        `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return nil, err
	}

	if time.Now().Unix()-cached.Timestamp > updateCacheTTL {
		return nil, fmt.Errorf("cache expired")
	}
	if cached.Repository != s.options.Repository || cached.DockerImage != s.options.DockerImage || cached.Channel != s.options.Channel {
		return nil, fmt.Errorf("cache source mismatch")
	}
	policy := cached.HotUpdatePolicy
	if policy != UpdatePolicySafe && policy != UpdatePolicyImageRecommended && policy != UpdatePolicyImageRequired {
		policy = UpdatePolicyImageRequired
		cached.HotUpdateReasons = []string{"cached release policy is invalid"}
	}
	if _, err := normalizeSourceCommit(cached.SourceCommit); err != nil && policy != UpdatePolicyImageRequired {
		policy = UpdatePolicyImageRequired
		cached.HotUpdateReasons = []string{"cached release source commit is invalid"}
	}

	return &UpdateInfo{
		CurrentVersion:   s.currentVersion,
		LatestVersion:    cached.Latest,
		HasUpdate:        compareVersions(s.currentVersion, cached.Latest) < 0,
		ReleaseInfo:      cached.ReleaseInfo,
		Cached:           true,
		BuildType:        s.buildType,
		Repository:       cached.Repository,
		DockerImage:      cached.DockerImage,
		Channel:          cached.Channel,
		HotUpdatePolicy:  policy,
		HotUpdateAllowed: s.hotUpdateAllowed(policy, cached.SourceCommit),
		HotUpdateReasons: cached.HotUpdateReasons,
		SourceCommit:     cached.SourceCommit,
	}, nil
}

func (s *UpdateService) saveToCache(ctx context.Context, info *UpdateInfo) {
	cacheData := struct {
		Repository       string       `json:"repository"`
		DockerImage      string       `json:"docker_image"`
		Channel          string       `json:"channel"`
		Latest           string       `json:"latest"`
		ReleaseInfo      *ReleaseInfo `json:"release_info"`
		HotUpdatePolicy  string       `json:"hot_update_policy"`
		HotUpdateAllowed bool         `json:"hot_update_allowed"`
		HotUpdateReasons []string     `json:"hot_update_reasons"`
		SourceCommit     string       `json:"source_commit"`
		Timestamp        int64        `json:"timestamp"`
	}{
		Repository:       s.options.Repository,
		DockerImage:      s.options.DockerImage,
		Channel:          s.options.Channel,
		Latest:           info.LatestVersion,
		ReleaseInfo:      info.ReleaseInfo,
		HotUpdatePolicy:  info.HotUpdatePolicy,
		HotUpdateAllowed: info.HotUpdateAllowed,
		HotUpdateReasons: info.HotUpdateReasons,
		SourceCommit:     info.SourceCommit,
		Timestamp:        time.Now().Unix(),
	}

	data, _ := json.Marshal(cacheData)
	_ = s.cache.SetUpdateInfo(ctx, string(data), time.Duration(updateCacheTTL)*time.Second, s.cacheNamespace())
}

func (s *UpdateService) cacheNamespace() string {
	return strings.Join([]string{
		s.options.Repository,
		s.options.DockerImage,
		s.options.Channel,
		s.buildType,
		strconv.FormatBool(s.options.InPlaceEnabled),
		strconv.FormatBool(s.options.RequireChecksum),
		strconv.FormatBool(s.options.RequireManifest),
		runtime.GOOS,
		runtime.GOARCH,
	}, "|")
}

// compareVersions compares semantic versions while treating the private
// -52t.N suffix as a stable revision of the same upstream release. This keeps
// the first private release (for example, 0.1.172-52t.1) newer than an
// existing upstream 0.1.172 installation instead of letting SemVer classify
// the private suffix as a prerelease.
func compareVersions(current, latest string) int {
	currentBase, currentRevision, currentPrivate := privateVersionOrder(current)
	latestBase, latestRevision, latestPrivate := privateVersionOrder(latest)
	if currentPrivate && latestPrivate {
		if comparison := semver.Compare(currentBase, latestBase); comparison != 0 {
			return comparison
		}
		return compareUint64(currentRevision, latestRevision)
	}
	if currentPrivate && !latestPrivate {
		if latestSemver := canonicalSemver(latest); latestSemver != "" && !strings.Contains(latestSemver, "-") {
			if comparison := semver.Compare(currentBase, latestSemver); comparison != 0 {
				return comparison
			}
			return compareUint64(currentRevision, 0)
		}
	}
	if !currentPrivate && latestPrivate {
		if currentSemver := canonicalSemver(current); currentSemver != "" && !strings.Contains(currentSemver, "-") {
			if comparison := semver.Compare(currentSemver, latestBase); comparison != 0 {
				return comparison
			}
			return compareUint64(0, latestRevision)
		}
	}

	current = canonicalSemver(current)
	latest = canonicalSemver(latest)
	if current == "" && latest == "" {
		return 0
	}
	if current == "" {
		return -1
	}
	if latest == "" {
		return 1
	}
	return semver.Compare(current, latest)
}

func privateVersionOrder(version string) (base string, revision uint64, ok bool) {
	canonical := canonicalSemver(version)
	if canonical == "" || !privateReleaseVersionPattern.MatchString(canonical) {
		return "", 0, false
	}
	parts := strings.SplitN(strings.TrimPrefix(canonical, "v"), "-52t.", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	parsedRevision, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return "v" + parts[0], parsedRevision, true
}

func compareUint64(left, right uint64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func canonicalSemver(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	if !semver.IsValid(version) {
		return ""
	}
	return version
}
