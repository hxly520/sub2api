package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	linkCardActivationGuardContextKey = "link_card_activation_guard"
	linkCardActivationFailureLimit    = int64(10)
	linkCardActivationFailureWindow   = 5 * time.Minute
	linkCardActivationLockDuration    = 5 * time.Minute
)

var linkCardActivationRecordScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if current == 1 or ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
if current >= tonumber(ARGV[1]) then
  redis.call('PEXPIRE', KEYS[1], ARGV[3])
end
return {current, redis.call('PTTL', KEYS[1])}
`)

type linkCardActivationGuardState struct {
	limiter   *LinkCardActivationLimiter
	clientKey string
}

// LinkCardActivationLimiter locks a trusted client IP after consecutive
// confirmed invalid quota-card credentials. State is shared through Redis so
// restarts and multiple replicas cannot be used to evade the lock.
type LinkCardActivationLimiter struct {
	redis *redis.Client
}

func NewLinkCardActivationLimiter(redisClient *redis.Client) *LinkCardActivationLimiter {
	return &LinkCardActivationLimiter{redis: redisClient}
}

func (l *LinkCardActivationLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientKey := linkCardActivationClientKey(c)
		if l == nil || l.redis == nil || clientKey == "" {
			abortLinkCardActivationGuardUnavailable(c)
			return
		}

		retryAfter, locked, err := l.check(c, clientKey)
		if err != nil {
			slog.Error("link card activation guard check failed", "error", err)
			abortLinkCardActivationGuardUnavailable(c)
			return
		}
		if locked {
			abortLinkCardActivationLocked(c, retryAfter)
			return
		}

		c.Set(linkCardActivationGuardContextKey, &linkCardActivationGuardState{
			limiter:   l,
			clientKey: clientKey,
		})
		c.Next()
	}
}

// HandleLinkCardActivationFailure records a confirmed unknown Key. It returns
// true when the middleware has already written a locked or fail-closed response.
func HandleLinkCardActivationFailure(c *gin.Context) bool {
	state, ok := linkCardActivationState(c)
	if !ok {
		return false
	}
	retryAfter, locked, err := state.limiter.recordFailure(c, state.clientKey)
	if err != nil {
		slog.Error("link card activation failure record failed", "error", err)
		abortLinkCardActivationGuardUnavailable(c)
		return true
	}
	if locked {
		abortLinkCardActivationLocked(c, retryAfter)
		return true
	}
	return false
}

// ResetLinkCardActivationFailures clears the consecutive-failure counter after
// a valid Key. A reset error is logged but cannot roll back an activated card.
func ResetLinkCardActivationFailures(c *gin.Context) {
	state, ok := linkCardActivationState(c)
	if !ok {
		return
	}
	if err := state.limiter.redis.Del(c.Request.Context(), state.clientKey).Err(); err != nil {
		slog.Error("link card activation failure reset failed", "error", err)
	}
}

func linkCardActivationState(c *gin.Context) (*linkCardActivationGuardState, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(linkCardActivationGuardContextKey)
	state, typed := value.(*linkCardActivationGuardState)
	return state, ok && typed && state != nil && state.limiter != nil && state.clientKey != ""
}

func (l *LinkCardActivationLimiter) check(c *gin.Context, key string) (time.Duration, bool, error) {
	count, err := l.redis.Get(c.Request.Context(), key).Int64()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if count < linkCardActivationFailureLimit {
		return 0, false, nil
	}
	ttl, err := l.redis.PTTL(c.Request.Context(), key).Result()
	if err != nil {
		return 0, false, err
	}
	if ttl <= 0 {
		if err := l.redis.Del(c.Request.Context(), key).Err(); err != nil {
			return 0, false, err
		}
		return 0, false, nil
	}
	return ttl, true, nil
}

func (l *LinkCardActivationLimiter) recordFailure(c *gin.Context, key string) (time.Duration, bool, error) {
	values, err := linkCardActivationRecordScript.Run(
		c.Request.Context(),
		l.redis,
		[]string{key},
		linkCardActivationFailureLimit,
		linkCardActivationFailureWindow.Milliseconds(),
		linkCardActivationLockDuration.Milliseconds(),
	).Slice()
	if err != nil {
		return 0, false, err
	}
	if len(values) != 2 {
		return 0, false, fmt.Errorf("link card activation guard returned %d values", len(values))
	}
	count, err := linkCardActivationInt64(values[0])
	if err != nil {
		return 0, false, err
	}
	ttlMillis, err := linkCardActivationInt64(values[1])
	if err != nil {
		return 0, false, err
	}
	if count < linkCardActivationFailureLimit {
		return 0, false, nil
	}
	retryAfter := time.Duration(ttlMillis) * time.Millisecond
	if retryAfter <= 0 {
		retryAfter = linkCardActivationLockDuration
	}
	return retryAfter, true, nil
}

func linkCardActivationClientKey(c *gin.Context) string {
	ip := strings.TrimSpace(normalizeIngressRejectIP(SecurityClientIP(c)))
	if ip == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(ip))
	return "security:link-card-activation:fail:" + hex.EncodeToString(digest[:16])
}

func linkCardActivationInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("unexpected link card activation guard value %T", value)
	}
}

func abortLinkCardActivationLocked(c *gin.Context, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = linkCardActivationLockDuration
	}
	retrySeconds := int64((retryAfter + time.Second - 1) / time.Second)
	c.Header("Retry-After", strconv.FormatInt(retrySeconds, 10))
	response.ErrorWithDetails(c, http.StatusTooManyRequests, "Too many invalid Key attempts; try again later", "LINK_CARD_ACTIVATION_LOCKED", map[string]string{
		"retry_after_seconds": strconv.FormatInt(retrySeconds, 10),
	})
	c.Abort()
}

func abortLinkCardActivationGuardUnavailable(c *gin.Context) {
	response.ErrorWithDetails(c, http.StatusServiceUnavailable, "Key validation is temporarily unavailable", "LINK_CARD_ACTIVATION_GUARD_UNAVAILABLE", nil)
	c.Abort()
}
