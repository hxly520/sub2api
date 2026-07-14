package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

// chatgptCodexModelsURL is the ChatGPT Codex models manifest endpoint.
// Package-level variable so tests can point it at a stub server.
var chatgptCodexModelsURL = "https://chatgpt.com/backend-api/codex/models"

const codexModelsManifestBodyLimit int64 = 8 << 20

var errNoAvailableCodexModelsAccount = errors.New("no available OpenAI accounts for Codex models manifest")

// CodexModelsAccountSelection describes how the models manifest should be
// served. APIKey-only groups cannot access ChatGPT's OAuth manifest; Codex can
// safely merge an empty remote catalog with its built-in catalog instead.
type CodexModelsAccountSelection struct {
	Account    *Account
	APIKeyOnly bool
}

// CodexModelsManifest carries the raw upstream manifest payload plus caching
// metadata so handlers can pass both through to the client untouched.
type CodexModelsManifest struct {
	Body        []byte
	ETag        string
	NotModified bool
}

// SelectAccountForCodexModels chooses a manifest-capable account without
// changing normal request scheduling. It deliberately prefers OAuth/setup-token
// accounts even when an API key relay has a higher scheduling priority.
func (s *OpenAIGatewayService) SelectAccountForCodexModels(ctx context.Context, groupID *int64) (*CodexModelsAccountSelection, error) {
	if s == nil || s.accountRepo == nil {
		return nil, errNoAvailableCodexModelsAccount
	}

	accounts, err := s.listSchedulableAccounts(ctx, groupID, PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	return s.selectAccountForCodexModels(ctx, accounts)
}

func (s *OpenAIGatewayService) selectAccountForCodexModels(ctx context.Context, accounts []Account) (*CodexModelsAccountSelection, error) {
	var firstBrokenOAuth *Account
	hasAPIKey := false

	for i := range accounts {
		candidate := s.resolveFreshSchedulableOpenAIAccount(ctx, &accounts[i], PlatformOpenAI, "", false, "")
		if candidate == nil {
			continue
		}
		candidate = s.recheckSelectedOpenAIAccountFromDB(ctx, candidate, PlatformOpenAI, "", false, "")
		if candidate == nil {
			continue
		}

		if candidate.Type == AccountTypeAPIKey {
			hasAPIKey = true
			continue
		}
		if !candidate.IsOAuth() {
			continue
		}

		credentialAccount, resolveErr := resolveCredentialAccount(ctx, s.accountRepo, candidate)
		if resolveErr == nil && credentialAccount != nil && strings.TrimSpace(credentialAccount.GetOpenAIAccessToken()) != "" {
			return &CodexModelsAccountSelection{Account: candidate}, nil
		}
		if firstBrokenOAuth == nil {
			firstBrokenOAuth = candidate
		}
	}

	// Preserve the existing 502 for a configured OAuth/setup-token account with
	// broken credentials. This keeps configuration faults visible instead of
	// silently treating a mixed group as APIKey-only.
	if firstBrokenOAuth != nil {
		return &CodexModelsAccountSelection{Account: firstBrokenOAuth}, nil
	}
	if hasAPIKey {
		return &CodexModelsAccountSelection{APIKeyOnly: true}, nil
	}
	return nil, errNoAvailableCodexModelsAccount
}

// FetchCodexModelsManifest fetches the live Codex models manifest from the
// ChatGPT backend using the account's OAuth credentials.
//
// The response body is passed through verbatim: the manifest schema evolves
// with Codex client releases, and interpreting it here would force the gateway
// to chase upstream changes. Passing it through keeps the gateway
// schema-agnostic and always reflects the account's real entitlements.
func (s *OpenAIGatewayService) FetchCodexModelsManifest(ctx context.Context, account *Account, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error) {
	if account == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_ACCOUNT_REQUIRED", "account is required")
	}
	credAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_CREDENTIALS_FAILED", "resolve credential account: %v", err)
	}
	accessToken := credAccount.GetOpenAIAccessToken()
	if accessToken == "" {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_MODELS_TOKEN_MISSING", "account has no Codex backend access token")
	}

	clientVersion = strings.TrimSpace(clientVersion)
	if clientVersion == "" {
		clientVersion = openAICodexProbeVersion
	}
	requestURL := chatgptCodexModelsURL + "?client_version=" + url.QueryEscape(clientVersion)

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_REQUEST_FAILED", "create codex models request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("Version", clientVersion)
	req.Header.Set("User-Agent", codexCLIUserAgent)
	if ifNoneMatch = strings.TrimSpace(ifNoneMatch); ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	setOpenAIChatGPTAccountHeaders(req.Header, credAccount)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:              proxyURL,
		Timeout:               15 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_CODEX_MODELS_PROXY_INVALID", "invalid proxy configuration: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return &CodexModelsManifest{ETag: resp.Header.Get("ETag"), NotModified: true}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "codex models manifest upstream error %d: %s", resp.StatusCode, message)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, codexModelsManifestBodyLimit))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_CODEX_MODELS_UPSTREAM_FAILED", "read codex models manifest response: %v", err)
	}
	return &CodexModelsManifest{Body: body, ETag: resp.Header.Get("ETag")}, nil
}
