package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

const (
	MediaGenerationStatusCreating  = "creating"
	MediaGenerationStatusPending   = "pending"
	MediaGenerationStatusRunning   = "running"
	MediaGenerationStatusCompleted = "completed"
	MediaGenerationStatusSucceeded = "succeeded"
	MediaGenerationStatusFailed    = "failed"
	MediaGenerationStatusCancelled = "cancelled"
	MediaGenerationStatusExpired   = "expired"
)

type MediaGenerationTask struct {
	ID int64
	// TaskID remains the legacy lookup key. New rows store the public task ID
	// here as well, while migrated rows retain their historical identifier.
	TaskID                 string
	PublicTaskID           string
	UpstreamTaskID         string
	APIKeyID               int64
	UserID                 int64
	AccountID              int64
	GroupID                *int64
	SubscriptionID         *int64
	Model                  string
	RequestedModel         string
	UpstreamModel          string
	Endpoint               string
	InboundEndpoint        string
	UpstreamEndpoint       string
	ChannelID              *int64
	ChannelMappedModel     string
	BillingModelSource     string
	ModelMappingChain      string
	RequestFingerprint     string
	RequestPayloadHash     string
	IdempotencyKeyHash     string
	ResponseStatus         int
	ResponseContentType    string
	ResponseBody           string
	UpstreamResultURL      string
	Status                 string
	DurationSeconds        int
	Resolution             string
	SizeTier               string
	BillingMode            string
	MediaType              string
	FinalizedAt            *time.Time
	FinalizationLeaseToken string
	FinalizationLeaseUntil *time.Time
	UsageRecordedAt        *time.Time
	FinalizationError      string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type MediaGenerationTaskRepository interface {
	GetMediaGenerationTaskByTaskID(ctx context.Context, apiKeyID int64, taskID string) (*MediaGenerationTask, error)
	GetMediaGenerationTaskByIdempotency(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (*MediaGenerationTask, error)
	AcquireMediaGenerationIdempotencyLock(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (func(), error)
	CreateMediaGenerationTask(ctx context.Context, task *MediaGenerationTask) error
	UpdateMediaGenerationTaskResponse(ctx context.Context, apiKeyID int64, taskID string, responseStatus int, responseContentType, responseBody, upstreamResultURL, status string, durationSeconds int) error
	MarkMediaGenerationTaskTerminal(ctx context.Context, apiKeyID int64, taskID, status, finalizationError string) error
	TryAcquireMediaGenerationFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken string, leaseUntil time.Time) (bool, error)
	CompleteMediaGenerationFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken string) (bool, error)
	ReleaseMediaGenerationFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken, finalizationError string) error
}

func (t *MediaGenerationTask) ClientTaskID() string {
	if t == nil {
		return ""
	}
	if value := strings.TrimSpace(t.PublicTaskID); value != "" {
		return value
	}
	return strings.TrimSpace(t.TaskID)
}

func (t *MediaGenerationTask) ProviderTaskID() string {
	if t == nil {
		return ""
	}
	if value := strings.TrimSpace(t.UpstreamTaskID); value != "" {
		return value
	}
	// Only pre-migration legacy rows may use task_id as the provider ID. New
	// rows always carry a public_task_id, and creation intents intentionally
	// have no provider ID until the upstream accepts the request.
	if strings.TrimSpace(t.PublicTaskID) == "" {
		return strings.TrimSpace(t.TaskID)
	}
	return ""
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
	case "fail", "failed", "failure", "error", "rejected", "denied", "aborted",
		"generation_failed", "prompt_blocked", "no_account", "content_policy_violation":
		return MediaGenerationStatusFailed
	case "cancel", "cancelled", "canceled":
		return MediaGenerationStatusCancelled
	case "expire", "expired", "timeout", "timed_out":
		return MediaGenerationStatusExpired
	case "creating", "created", "initializing", "submitting", "submitted", "not_started":
		return MediaGenerationStatusCreating
	case "queued", "queueing", "in_queue", "waiting", "scheduled", "pending":
		return MediaGenerationStatusPending
	case "running", "processing", "in_progress", "generating", "cancelling", "canceling":
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
