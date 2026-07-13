package handler

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const mediaUsageRecordMaxAttempts = 3

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

func releaseMediaBalanceHold(h *OpenAIGatewayHandler, cmd *service.MediaBalanceHoldCommand) {
	if h == nil || h.gatewayService == nil || cmd == nil || strings.TrimSpace(cmd.RequestID) == "" || cmd.HoldAmount <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.gatewayService.ReleaseMediaBalance(ctx, cmd)
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

func mediaBalanceHoldCommandForTask(task *service.MediaGenerationTask) *service.MediaBalanceHoldCommand {
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
	return newMediaBalanceHoldCommand(
		task.APIKeyID,
		task.UserID,
		service.MediaBalanceHoldRequestID(task.ClientTaskID()),
		task.RequestFingerprint,
		task.RequestPayloadHash,
		amount,
	)
}
