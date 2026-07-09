package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const (
	MediaGenerationStatusPending   = "pending"
	MediaGenerationStatusRunning   = "running"
	MediaGenerationStatusCompleted = "completed"
	MediaGenerationStatusSucceeded = "succeeded"
	MediaGenerationStatusFailed    = "failed"
	MediaGenerationStatusCancelled = "cancelled"
	MediaGenerationStatusExpired   = "expired"
)

type MediaGenerationTask struct {
	ID                  int64
	TaskID              string
	APIKeyID            int64
	UserID              int64
	AccountID           int64
	GroupID             *int64
	SubscriptionID      *int64
	Model               string
	RequestedModel      string
	UpstreamModel       string
	Endpoint            string
	InboundEndpoint     string
	UpstreamEndpoint    string
	ChannelID           *int64
	ChannelMappedModel  string
	BillingModelSource  string
	ModelMappingChain   string
	RequestFingerprint  string
	RequestPayloadHash  string
	IdempotencyKeyHash  string
	ResponseStatus      int
	ResponseContentType string
	ResponseBody        string
	Status              string
	DurationSeconds     int
	Resolution          string
	SizeTier            string
	BillingMode         string
	MediaType           string
	FinalizedAt         *time.Time
	FinalizationError   string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type MediaGenerationTaskRepository interface {
	GetMediaGenerationTaskByTaskID(ctx context.Context, apiKeyID int64, taskID string) (*MediaGenerationTask, error)
	GetMediaGenerationTaskByIdempotency(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (*MediaGenerationTask, error)
	AcquireMediaGenerationIdempotencyLock(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (func(), error)
	CreateMediaGenerationTask(ctx context.Context, task *MediaGenerationTask) error
	UpdateMediaGenerationTaskResponse(ctx context.Context, apiKeyID int64, taskID string, responseStatus int, responseContentType, responseBody, status string, durationSeconds int) error
	MarkMediaGenerationTaskTerminal(ctx context.Context, apiKeyID int64, taskID, status, finalizationError string) error
}

func HashMediaGenerationIdempotencyKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func HashMediaGenerationRequestFingerprint(endpoint string, payload []byte) string {
	seed := strings.TrimSpace(endpoint) + "\n" + string(payload)
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func NormalizeMediaGenerationStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "complete", "completed", "success", "succeeded", "done":
		return MediaGenerationStatusCompleted
	case "fail", "failed", "error":
		return MediaGenerationStatusFailed
	case "cancel", "cancelled", "canceled":
		return MediaGenerationStatusCancelled
	case "expire", "expired", "timeout", "timed_out":
		return MediaGenerationStatusExpired
	case "running", "processing", "in_progress", "generating":
		return MediaGenerationStatusRunning
	default:
		if strings.TrimSpace(status) == "" {
			return MediaGenerationStatusPending
		}
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func IsMediaGenerationSuccessStatus(status string) bool {
	return NormalizeMediaGenerationStatus(status) == MediaGenerationStatusCompleted
}

func IsMediaGenerationFailureStatus(status string) bool {
	switch NormalizeMediaGenerationStatus(status) {
	case MediaGenerationStatusFailed, MediaGenerationStatusCancelled, MediaGenerationStatusExpired:
		return true
	default:
		return false
	}
}
