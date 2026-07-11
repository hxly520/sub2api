package service

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/sync/singleflight"
)

const openAIFirstResponseSettingCacheKey = "openai_first_response"

const (
	openAIFirstResponseTimeoutMinMS     = 500
	openAIFirstResponseTimeoutMaxMS     = 60000
	openAIFirstResponseTimeoutDefaultMS = 5000
	openAIFirstResponseMaxAttemptsMin   = 1

	openAIFirstResponseSettingCacheTTL  = 5 * time.Second
	openAIFirstResponseSettingDBTimeout = 2 * time.Second
)

type OpenAIFirstResponseRuntimeConfig struct {
	Enabled      bool
	TimeoutMS    int
	MaxAttempts  int
	CountAsError bool
}

type cachedOpenAIFirstResponseRuntimeConfig struct {
	config    OpenAIFirstResponseRuntimeConfig
	expiresAt int64
}

var openAIFirstResponseRuntimeConfigCache atomic.Value // *cachedOpenAIFirstResponseRuntimeConfig
var openAIFirstResponseRuntimeConfigSF singleflight.Group

func defaultOpenAIFirstResponseRuntimeConfigFromConfig(cfg *config.Config) OpenAIFirstResponseRuntimeConfig {
	out := OpenAIFirstResponseRuntimeConfig{
		TimeoutMS:   openAIFirstResponseTimeoutDefaultMS,
		MaxAttempts: 2,
	}
	if cfg == nil {
		return out
	}
	out.Enabled = cfg.Gateway.OpenAIFirstResponse.Enabled
	out.TimeoutMS = normalizeOpenAIFirstResponseTimeoutMSValue(
		cfg.Gateway.OpenAIFirstResponse.TimeoutMS,
		openAIFirstResponseTimeoutDefaultMS,
	)
	if cfg.Gateway.OpenAIFirstResponse.MaxAttempts >= openAIFirstResponseMaxAttemptsMin {
		out.MaxAttempts = cfg.Gateway.OpenAIFirstResponse.MaxAttempts
	}
	out.CountAsError = cfg.Gateway.OpenAIFirstResponse.CountAsError
	return out
}

func normalizeOpenAIFirstResponseTimeoutMSValue(value int, fallback int) int {
	if fallback <= 0 {
		fallback = openAIFirstResponseTimeoutDefaultMS
	}
	if value < openAIFirstResponseTimeoutMinMS || value > openAIFirstResponseTimeoutMaxMS {
		return fallback
	}
	return value
}

func parseOpenAIFirstResponseTimeoutMS(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return normalizeOpenAIFirstResponseTimeoutMSValue(value, fallback)
}

func validateOpenAIFirstResponseTimeoutMS(timeoutMS int) error {
	if timeoutMS < openAIFirstResponseTimeoutMinMS || timeoutMS > openAIFirstResponseTimeoutMaxMS {
		return infraerrors.BadRequest(
			"INVALID_OPENAI_FIRST_RESPONSE_TIMEOUT",
			"openai_first_response_timeout_ms must be between 500 and 60000 milliseconds",
		)
	}
	return nil
}

func (s *OpenAIGatewayService) openAIFirstResponseSettingRepo() SettingRepository {
	if s == nil {
		return nil
	}
	if s.settingService != nil {
		return s.settingService.settingRepo
	}
	if s.rateLimitService != nil && s.rateLimitService.settingService != nil {
		return s.rateLimitService.settingService.settingRepo
	}
	return nil
}

func (s *OpenAIGatewayService) GetOpenAIFirstResponseRuntimeConfig(ctx context.Context) OpenAIFirstResponseRuntimeConfig {
	fallback := defaultOpenAIFirstResponseRuntimeConfigFromConfig(nil)
	if s != nil {
		fallback = defaultOpenAIFirstResponseRuntimeConfigFromConfig(s.cfg)
	}
	if cached, ok := openAIFirstResponseRuntimeConfigCache.Load().(*cachedOpenAIFirstResponseRuntimeConfig); ok && cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cached.config
	}
	if ctx == nil {
		ctx = context.Background()
	}

	result, _, _ := openAIFirstResponseRuntimeConfigSF.Do(openAIFirstResponseSettingCacheKey, func() (any, error) {
		if cached, ok := openAIFirstResponseRuntimeConfigCache.Load().(*cachedOpenAIFirstResponseRuntimeConfig); ok && cached != nil && time.Now().UnixNano() < cached.expiresAt {
			return cached.config, nil
		}

		runtimeCfg := fallback
		if repo := s.openAIFirstResponseSettingRepo(); repo != nil {
			dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIFirstResponseSettingDBTimeout)
			defer cancel()

			if raw, err := repo.GetValue(dbCtx, SettingKeyOpenAIFirstResponseEnabled); err == nil {
				runtimeCfg.Enabled = strings.EqualFold(strings.TrimSpace(raw), "true")
			}
			if raw, err := repo.GetValue(dbCtx, SettingKeyOpenAIFirstResponseTimeoutMS); err == nil {
				runtimeCfg.TimeoutMS = parseOpenAIFirstResponseTimeoutMS(raw, fallback.TimeoutMS)
			}
		}

		openAIFirstResponseRuntimeConfigCache.Store(&cachedOpenAIFirstResponseRuntimeConfig{
			config:    runtimeCfg,
			expiresAt: time.Now().Add(openAIFirstResponseSettingCacheTTL).UnixNano(),
		})
		return runtimeCfg, nil
	})

	if runtimeCfg, ok := result.(OpenAIFirstResponseRuntimeConfig); ok {
		return runtimeCfg
	}
	return fallback
}

func invalidateOpenAIFirstResponseRuntimeConfigCache() {
	openAIFirstResponseRuntimeConfigSF.Forget(openAIFirstResponseSettingCacheKey)
	openAIFirstResponseRuntimeConfigCache.Store(&cachedOpenAIFirstResponseRuntimeConfig{expiresAt: 0})
}
