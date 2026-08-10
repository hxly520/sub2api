//go:build unit

package service

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	updateTestSourceCommit      = "0123456789abcdef0123456789abcdef01234567"
	updateTestOtherSourceCommit = "89abcdef0123456789abcdef0123456789abcdef"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context, ...string) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration, _ ...string) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release        *GitHubRelease
	latestErr      error
	recentReleases []*GitHubRelease
	recentErr      error
	checksumData   []byte
	checksumErr    error
	fetchedURLs    []string
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	return s.release, s.latestErr
}

func (s *updateServiceGitHubClientStub) FetchRecentReleases(context.Context, string, int) ([]*GitHubRelease, error) {
	return s.recentReleases, s.recentErr
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(_ context.Context, rawURL string) ([]byte, error) {
	s.fetchedURLs = append(s.fetchedURLs, rawURL)
	return s.checksumData, s.checksumErr
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132-52t.1",
				Name:    "v0.1.132-52t.1",
			},
		},
		"0.1.132-52t.1",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func newRollbackTestService(current string, releases []*GitHubRelease) *UpdateService {
	return NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentReleases: releases},
		current,
		"release",
	)
}

func TestUpdateServiceListRollbackVersionsFiltersAndCaps(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148-52t.1", PublishedAt: "2026-07-09T00:00:00Z"},                   // newer than current: excluded
		{TagName: "v0.1.147-52t.2", PublishedAt: "2026-07-08T00:00:00Z"},                   // current: excluded
		{TagName: "v0.1.146-52t.1", PublishedAt: "2026-07-07T12:00:00Z", Prerelease: true}, // GitHub prerelease: excluded
		{TagName: "v0.1.146-52t.1", PublishedAt: "2026-07-07T00:00:00Z"},
		{TagName: "v0.1.145-52t.1", PublishedAt: "2026-07-06T00:00:00Z", Draft: true}, // draft: excluded
		{TagName: "v0.1.144-52t.3", PublishedAt: "2026-07-05T00:00:00Z"},
		{TagName: "v0.1.144-52t.3", PublishedAt: "2026-07-05T00:00:00Z"}, // duplicate: excluded
		{TagName: "v0.1.143-52t.1", PublishedAt: "2026-07-04T00:00:00Z"},
		{TagName: "v0.1.142-52t.1", PublishedAt: "2026-07-03T00:00:00Z"}, // beyond cap of 3: excluded
		{TagName: "v0.1.141", PublishedAt: "2026-07-02T00:00:00Z"},       // upstream tag: excluded
	}
	svc := newRollbackTestService("0.1.147-52t.2", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146-52t.1", versions[0].Version)
	require.Equal(t, "0.1.144-52t.3", versions[1].Version)
	require.Equal(t, "0.1.143-52t.1", versions[2].Version)
	require.Equal(t, UpdatePolicyImageRequired, versions[0].HotUpdatePolicy)
	require.False(t, versions[0].HotUpdateAllowed)
}

func TestUpdateServiceListRollbackVersionsSortsUnorderedInput(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.144-52t.1"},
		{TagName: "v0.1.146-52t.1"},
		{TagName: "v0.1.145-52t.1"},
	}
	svc := newRollbackTestService("0.1.147-52t.1", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Len(t, versions, 3)
	require.Equal(t, "0.1.146-52t.1", versions[0].Version)
	require.Equal(t, "0.1.145-52t.1", versions[1].Version)
	require.Equal(t, "0.1.144-52t.1", versions[2].Version)
}

func TestUpdateServiceListRollbackVersionsEmptyWhenNoneOlder(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.147-52t.1"},
		{TagName: "v0.1.148-52t.1"},
	}
	svc := newRollbackTestService("0.1.147-52t.1", releases)

	versions, err := svc.ListRollbackVersions(context.Background())

	require.NoError(t, err)
	require.Empty(t, versions)
}

func TestUpdateServiceListRollbackVersionsPropagatesFetchError(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{recentErr: errors.New("github unavailable")},
		"0.1.147-52t.1",
		"release",
	)

	_, err := svc.ListRollbackVersions(context.Background())

	require.Error(t, err)
	require.Contains(t, err.Error(), "github unavailable")
}

func TestUpdateServiceRollbackToVersionRejectsDisallowedTargets(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.148-52t.1"},
		{TagName: "v0.1.147-52t.1"},
		{TagName: "v0.1.146-52t.1"},
		{TagName: "v0.1.145-52t.1"},
		{TagName: "v0.1.144-52t.1"},
		{TagName: "v0.1.143-52t.1"},
		{TagName: "v0.1.142-52t.1"},
	}
	svc := newRollbackTestService("0.1.147-52t.1", releases)

	for _, target := range []string{
		"",               // empty
		"0.1.147-52t.1",  // current version
		"v0.1.147-52t.1", // current version with prefix
		"0.1.148-52t.1",  // newer than current
		"0.1.142-52t.1",  // older than the 3 most recent
		"9.9.9",          // nonexistent
	} {
		err := svc.RollbackToVersion(context.Background(), target)
		require.ErrorIs(t, err, ErrRollbackVersionNotAllowed, "target %q should be rejected", target)
	}
}

func TestUpdateServiceRollbackToVersionAcceptsVPrefix(t *testing.T) {
	// No platform asset in the release: the target passes the allowlist check
	// and fails later at asset lookup, proving the version itself was accepted.
	releases := []*GitHubRelease{
		{TagName: "v0.1.147-52t.1"},
		{
			TagName: "v0.1.146-52t.1",
			Assets: []GitHubAsset{{
				Name:   updateManifestAssetName,
				APIURL: "https://api.github.com/repos/hxly520/sub2api/releases/assets/41",
			}},
		},
	}
	client := &updateServiceGitHubClientStub{
		recentReleases: releases,
		checksumData:   []byte(`{"schema_version":1,"version":"0.1.146-52t.1","source_commit":"` + updateTestSourceCommit + `","policy":"hot-update-safe","reasons":[]}`),
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.147-52t.1", "release")

	err := svc.RollbackToVersion(context.Background(), "v0.1.146-52t.1")

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRollbackVersionNotAllowed)
	if runtime.GOOS == "linux" {
		require.Contains(t, err.Error(), "no compatible release found")
	} else {
		require.ErrorIs(t, err, ErrImageUpdateRequired)
	}
}

func TestUpdateServiceRollbackImagePolicyRequiresCompose(t *testing.T) {
	releases := []*GitHubRelease{
		{TagName: "v0.1.173-52t.2"},
		{TagName: "v0.1.173-52t.1"},
	}
	svc := newRollbackTestService("0.1.173-52t.2", releases)

	err := svc.RollbackToVersion(context.Background(), "v0.1.173-52t.1")

	require.ErrorIs(t, err, ErrImageUpdateRequired)
}

func TestUpdateServiceRejectsUpstreamLatestTag(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.1.174"}},
		"0.1.173-52t.1",
		"release",
	)

	_, err := svc.CheckUpdate(context.Background(), true)

	require.ErrorContains(t, err, "not a private vX.Y.Z-52t.N version")
}

func TestNormalizePrivateReleaseVersionRejectsNonCanonicalTags(t *testing.T) {
	for _, tag := range []string{
		"0.1.173-52t.1",
		"v01.1.173-52t.1",
		"v0.01.173-52t.1",
		"v0.1.0173-52t.1",
		"v0.1.173-52t.01",
	} {
		_, err := normalizePrivateReleaseVersion(tag)
		require.Error(t, err, tag)
	}
}

func TestUpdateServiceTreatsFirstPrivateReleaseAsUpdate(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{release: &GitHubRelease{TagName: "v0.1.172-52t.1"}},
		"0.1.172",
		"release",
	)

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	require.Equal(t, "0.1.172-52t.1", info.LatestVersion)
	require.Equal(t, UpdatePolicyImageRequired, info.HotUpdatePolicy)
}

func TestValidateDownloadURLAllowsOnlyExactGitHubAssetHosts(t *testing.T) {
	for _, rawURL := range []string{
		"https://api.github.com/repos/hxly520/sub2api/releases/assets/1",
		"https://github.com/hxly520/sub2api/releases/download/v1/asset",
		"https://objects.githubusercontent.com/asset",
		"https://release-assets.githubusercontent.com/asset",
	} {
		require.NoError(t, validateDownloadURL(rawURL), rawURL)
	}

	for _, rawURL := range []string{
		"http://api.github.com/repos/hxly520/sub2api/releases/assets/1",
		"https://user@api.github.com/repos/hxly520/sub2api/releases/assets/1",
		"https://api.github.com:443/repos/hxly520/sub2api/releases/assets/1",
		"https://uploads.github.com/asset",
		"https://github.com.example.invalid/asset",
	} {
		require.Error(t, validateDownloadURL(rawURL), rawURL)
	}
}

func TestUpdateServiceCacheNamespaceIncludesDeploymentPolicy(t *testing.T) {
	base := DefaultUpdateOptions()
	first := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.173-52t.1", "release", base)
	changed := base
	changed.InPlaceEnabled = false
	second := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.173-52t.1", "release", changed)

	require.NotEqual(t, first.cacheNamespace(), second.cacheNamespace())
}

func TestCompareVersionsSupportsPrivateSemverRevisions(t *testing.T) {
	require.Less(t, compareVersions("0.1.173-52t.1", "0.1.173-52t.2"), 0)
	require.Greater(t, compareVersions("0.1.174-52t.1", "0.1.173-52t.99"), 0)
	require.Equal(t, 0, compareVersions("v0.1.173-52t.2", "0.1.173-52t.2"))
	require.Less(t, compareVersions("invalid", "0.1.173-52t.1"), 0)
	// A plain upstream build is the implicit private revision zero, so the
	// first private release is visible as an update during migration.
	require.Less(t, compareVersions("0.1.172", "0.1.172-52t.1"), 0)
	require.Greater(t, compareVersions("0.1.172-52t.1", "0.1.172"), 0)
}

func TestUpdateServiceForcedCheckDoesNotFallBackToCache(t *testing.T) {
	cache := &updateServiceCacheStub{data: `{
		"repository":"hxly520/sub2api",
		"channel":"stable",
		"latest":"9.9.9",
		"timestamp":4102444800
	}`}
	svc := NewUpdateService(cache, &updateServiceGitHubClientStub{latestErr: errors.New("private release unavailable")}, "0.1.173-52t.1", "release")

	_, err := svc.CheckUpdate(context.Background(), true)

	require.ErrorContains(t, err, "private release unavailable")
}

func TestUpdateServiceMissingManifestRequiresCompose(t *testing.T) {
	client := &updateServiceGitHubClientStub{release: &GitHubRelease{
		TagName: "v0.1.173-52t.2",
		Name:    "private release",
	}}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.173-52t.1", "release")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	require.False(t, info.HotUpdateAllowed)
	require.Equal(t, UpdatePolicyImageRequired, info.HotUpdatePolicy)
	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrImageUpdateRequired)
}

func TestUpdateServiceValidManifestAllowsHotUpdate(t *testing.T) {
	client := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v0.1.173-52t.2",
			Name:    "private release",
			Assets: []GitHubAsset{{
				Name:   updateManifestAssetName,
				APIURL: "https://api.github.com/repos/hxly520/sub2api/releases/assets/42",
			}},
		},
		checksumData: []byte(`{"schema_version":1,"version":"0.1.173-52t.2","source_commit":"` + updateTestSourceCommit + `","policy":"hot-update-safe","reasons":[]}`),
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.173-52t.1", "release")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	if runtime.GOOS == "linux" {
		require.True(t, info.HotUpdateAllowed)
	} else {
		require.False(t, info.HotUpdateAllowed)
	}
	require.Equal(t, UpdatePolicySafe, info.HotUpdatePolicy)
	require.Equal(t, updateTestSourceCommit, info.SourceCommit)
	require.Equal(t, []string{"https://api.github.com/repos/hxly520/sub2api/releases/assets/42"}, client.fetchedURLs)
}

func TestUpdateServiceMalformedManifestCommitRequiresCompose(t *testing.T) {
	client := &updateServiceGitHubClientStub{
		release: &GitHubRelease{
			TagName: "v0.1.173-52t.2",
			Assets: []GitHubAsset{{
				Name:   updateManifestAssetName,
				APIURL: "https://api.github.com/repos/hxly520/sub2api/releases/assets/42",
			}},
		},
		checksumData: []byte(`{"schema_version":1,"version":"0.1.173-52t.2","source_commit":"abc","policy":"hot-update-safe","reasons":[]}`),
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, client, "0.1.173-52t.1", "release")

	info, err := svc.CheckUpdate(context.Background(), true)

	require.NoError(t, err)
	require.Equal(t, UpdatePolicyImageRequired, info.HotUpdatePolicy)
	require.False(t, info.HotUpdateAllowed)
	require.Empty(t, info.SourceCommit)
	require.Contains(t, info.HotUpdateReasons[0], "source_commit must be a full 40-character hexadecimal commit")
	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrImageUpdateRequired)
}

func TestValidateReplacementVersionOutputMatchesManifestCommit(t *testing.T) {
	output := []byte("2026/08/11 10:30:00 Sub2API 0.1.173-52t.2 (commit: " + updateTestSourceCommit + ", built: 2026-08-11T02:30:00Z)")

	require.NoError(t, validateReplacementVersionOutput(output, "0.1.173-52t.2", updateTestSourceCommit))
	require.ErrorContains(
		t,
		validateReplacementVersionOutput(output, "0.1.173-52t.2", updateTestOtherSourceCommit),
		"does not match manifest source commit",
	)
	require.ErrorContains(
		t,
		validateReplacementVersionOutput(output, "0.1.173-52t.2", "abc"),
		"must be a full 40-character hexadecimal commit",
	)
}

func TestUpdateServiceSourceBuildRejectsInPlaceUpdate(t *testing.T) {
	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.173-52t.1", "source")

	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrInPlaceUpdateDisabled)
}

func TestUpdateServiceLocalRollbackRejectsNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-Linux platform guard")
	}
	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.173-52t.1", "release")

	require.ErrorIs(t, svc.Rollback(), ErrInPlaceUpdateDisabled)
}

func TestUpdateServiceLocalRollbackHonorsInPlacePolicy(t *testing.T) {
	options := DefaultUpdateOptions()
	options.InPlaceEnabled = false
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{},
		"0.1.173-52t.1",
		"release",
		options,
	)

	require.ErrorIs(t, svc.Rollback(), ErrInPlaceUpdateDisabled)
}

func TestUpdateServiceSourceBuildRejectsLocalRollback(t *testing.T) {
	svc := NewUpdateService(&updateServiceCacheStub{}, &updateServiceGitHubClientStub{}, "0.1.173-52t.1", "source")

	require.ErrorIs(t, svc.Rollback(), ErrInPlaceUpdateDisabled)
}
