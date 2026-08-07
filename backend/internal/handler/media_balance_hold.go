package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

const mediaUsageRecordMaxAttempts = 3

func shouldRetainMediaBalanceHoldAfterDispatch(err error) bool {
	if err == nil {
		return false
	}
	if service.IsMarkedDefinitiveMediaGenerationFailure(err) {
		return false
	}
	var upstreamUserErr *service.OpenAIImagesUpstreamError
	if errors.As(err, &upstreamUserErr) {
		if upstreamUserErr.MediaOutcomeKnownFailed {
			return false
		}
		return !service.IsDefinitiveMediaGenerationFailure(upstreamUserErr.StatusCode, []byte(upstreamUserErr.Error()))
	}
	var failoverErr *service.UpstreamFailoverError
	if errors.As(err, &failoverErr) {
		return !failoverErr.MediaOutcomeKnownFailed
	}
	return true
}

func newMediaBalanceHoldCommand(apiKeyID, userID int64, requestID, requestFingerprint, payloadHash string, amount float64) *service.MediaBalanceHoldCommand {
	return &service.MediaBalanceHoldCommand{
		RequestID:          strings.TrimSpace(requestID),
		APIKeyID:           apiKeyID,
		UserID:             userID,
		RequestFingerprint: strings.TrimSpace(requestFingerprint),
		RequestPayloadHash: strings.TrimSpace(payloadHash),
		HoldAmount:         amount,
	}
}

func markLinkCardMediaBalanceHold(cmd *service.MediaBalanceHoldCommand, apiKey *service.APIKey) *service.MediaBalanceHoldCommand {
	if cmd != nil && apiKey != nil {
		cmd.LinkCard = apiKey.IsLinkKey()
	}
	return cmd
}

func releaseMediaBalanceHold(h *OpenAIGatewayHandler, cmd *service.MediaBalanceHoldCommand) {
	if h == nil || h.gatewayService == nil || cmd == nil || strings.TrimSpace(cmd.RequestID) == "" || cmd.HoldAmount <= 0 {
		return
	}
	var lastErr error
	for attempt := 0; attempt < mediaUsageRecordMaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		lastErr = h.gatewayService.ReleaseMediaBalance(ctx, cmd)
		cancel()
		if lastErr == nil {
			return
		}
		if attempt+1 < mediaUsageRecordMaxAttempts {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}
	logger.L().Error("media_balance_hold.release_failed",
		zap.String("request_id", cmd.RequestID),
		zap.Int64("user_id", cmd.UserID),
		zap.Int64("api_key_id", cmd.APIKeyID),
		zap.Float64("hold_amount", cmd.HoldAmount),
		zap.Error(lastErr),
	)
}

func markMediaBalanceHoldForCapture(h *OpenAIGatewayHandler, cmd *service.MediaBalanceHoldCommand, actualCost float64) error {
	if h == nil || h.gatewayService == nil || cmd == nil || strings.TrimSpace(cmd.RequestID) == "" || cmd.HoldAmount <= 0 {
		return nil
	}
	if actualCost <= 0 {
		actualCost = cmd.HoldAmount
	}
	var lastErr error
	for attempt := 0; attempt < mediaUsageRecordMaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		lastErr = h.gatewayService.MarkMediaBalanceForCapture(ctx, cmd, actualCost)
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt+1 < mediaUsageRecordMaxAttempts {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}
	return lastErr
}

func markMediaBalanceHoldDispatched(h *OpenAIGatewayHandler, cmd *service.MediaBalanceHoldCommand) error {
	if h == nil || h.gatewayService == nil || cmd == nil || strings.TrimSpace(cmd.RequestID) == "" || cmd.HoldAmount <= 0 {
		return nil
	}
	var lastErr error
	for attempt := 0; attempt < mediaUsageRecordMaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		lastErr = h.gatewayService.MarkMediaBalanceDispatched(ctx, cmd)
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt+1 < mediaUsageRecordMaxAttempts {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}
	return lastErr
}

func recordMediaUsageWithRetry(record func(context.Context) error) error {
	if record == nil {
		return nil
	}
	var lastErr error
	for attempt := 0; attempt < mediaUsageRecordMaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		lastErr = record(ctx)
		cancel()
		if lastErr == nil {
			return nil
		}
		if attempt+1 < mediaUsageRecordMaxAttempts {
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
		}
	}
	return lastErr
}

func mediaBalanceActualCost(snapshot *service.MediaGenerationPricingSnapshot, result *service.OpenAIForwardResult, fallbackCount, fallbackDuration int, video bool) float64 {
	if snapshot == nil {
		return 0
	}
	count := fallbackCount
	duration := fallbackDuration
	if result != nil {
		if video {
			if result.VideoCount > 0 {
				count = result.VideoCount
			}
			if result.VideoDurationSeconds > 0 {
				duration = result.VideoDurationSeconds
			} else if result.MediaDurationSeconds > 0 {
				duration = result.MediaDurationSeconds
			}
		} else if result.ImageCount > 0 {
			count = result.ImageCount
		}
	}
	return snapshot.EstimatedCost(count, duration)
}

func mediaBalanceSettledCost(cmd *service.MediaBalanceHoldCommand, actualCost float64) float64 {
	if cmd == nil || cmd.HoldAmount <= 0 || actualCost <= cmd.HoldAmount {
		return actualCost
	}
	return cmd.HoldAmount
}

func mediaBalanceHoldCommandForTask(task *service.MediaGenerationTask, keys ...*service.APIKey) *service.MediaBalanceHoldCommand {
	if task == nil || task.APIKeyID <= 0 || task.UserID <= 0 || strings.TrimSpace(task.ClientTaskID()) == "" {
		return nil
	}
	snapshot := task.PricingSnapshot()
	if snapshot == nil {
		return nil
	}
	count := task.RequestCount
	if count <= 0 {
		count = 1
	}
	amount := snapshot.EstimatedCost(count, task.DurationSeconds)
	if amount <= 0 {
		return nil
	}
	cmd := newMediaBalanceHoldCommand(
		task.APIKeyID,
		task.UserID,
		service.MediaBalanceHoldRequestID(task.ClientTaskID()),
		task.RequestFingerprint,
		task.RequestPayloadHash,
		amount,
	)
	if len(keys) > 0 {
		markLinkCardMediaBalanceHold(cmd, keys[0])
	}
	return cmd
}
