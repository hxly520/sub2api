package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
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
	BillingUnitPrice       *float64
	BillingRateMultiplier  *float64
	MediaType              string
	FinalizedAt            *time.Time
	FinalizationLeaseToken string
	FinalizationLeaseUntil *time.Time
	UsageRecordedAt        *time.Time
	FinalizationError      string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type MediaGenerationPricingSnapshot struct {
	Mode           BillingMode
	UnitPrice      float64
	RateMultiplier float64
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

func (s *OpenAIGatewayService) GetMediaGenerationTaskByTaskID(ctx context.Context, apiKeyID int64, taskID string) (*MediaGenerationTask, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.GetMediaGenerationTaskByTaskID(ctx, apiKeyID, taskID)
}

func (s *OpenAIGatewayService) GetMediaGenerationTaskByIdempotency(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (*MediaGenerationTask, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.GetMediaGenerationTaskByIdempotency(ctx, apiKeyID, idempotencyKeyHash)
}

func (s *OpenAIGatewayService) AcquireMediaGenerationIdempotencyLock(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (func(), error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.AcquireMediaGenerationIdempotencyLock(ctx, apiKeyID, idempotencyKeyHash)
}

func (s *OpenAIGatewayService) CreateMediaGenerationTask(ctx context.Context, task *MediaGenerationTask) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.CreateMediaGenerationTask(ctx, task)
}

func (s *OpenAIGatewayService) UpdateMediaGenerationTaskResponse(
	ctx context.Context,
	apiKeyID int64,
	taskID string,
	responseStatus int,
	responseContentType string,
	responseBody []byte,
	upstreamResultURL string,
	status string,
	durationSeconds int,
) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.UpdateMediaGenerationTaskResponse(
		ctx,
		apiKeyID,
		taskID,
		responseStatus,
		responseContentType,
		string(responseBody),
		upstreamResultURL,
		status,
		durationSeconds,
	)
}

func (s *OpenAIGatewayService) MarkMediaGenerationTaskTerminal(ctx context.Context, apiKeyID int64, taskID, status, finalizationError string) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.MarkMediaGenerationTaskTerminal(ctx, apiKeyID, taskID, status, finalizationError)
}

func (s *OpenAIGatewayService) TryAcquireMediaGenerationFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken string, leaseUntil time.Time) (bool, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return false, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.TryAcquireMediaGenerationFinalization(ctx, apiKeyID, taskID, leaseToken, leaseUntil)
}

func (s *OpenAIGatewayService) CompleteMediaGenerationFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken string) (bool, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return false, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.CompleteMediaGenerationFinalization(ctx, apiKeyID, taskID, leaseToken)
}

func (s *OpenAIGatewayService) ReleaseMediaGenerationFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken, finalizationError string) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.ReleaseMediaGenerationFinalization(ctx, apiKeyID, taskID, leaseToken, finalizationError)
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

func (t *MediaGenerationTask) PricingSnapshot() *MediaGenerationPricingSnapshot {
	if t == nil || t.BillingUnitPrice == nil || t.BillingRateMultiplier == nil {
		return nil
	}
	mode := BillingMode(strings.TrimSpace(t.BillingMode))
	if mode != BillingModePerRequest && mode != BillingModeImage && mode != BillingModeVideo {
		return nil
	}
	unitPrice := *t.BillingUnitPrice
	rateMultiplier := *t.BillingRateMultiplier
	if unitPrice < 0 || rateMultiplier < 0 || math.IsNaN(unitPrice) || math.IsNaN(rateMultiplier) ||
		math.IsInf(unitPrice, 0) || math.IsInf(rateMultiplier, 0) {
		return nil
	}
	return &MediaGenerationPricingSnapshot{
		Mode:           mode,
		UnitPrice:      unitPrice,
		RateMultiplier: rateMultiplier,
	}
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
