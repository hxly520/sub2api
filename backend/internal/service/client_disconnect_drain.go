package service

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

// clientDisconnectDrainFallbackTimeout is deliberately short.  Once the
// downstream has gone away, usage can still arrive in a few seconds, but a
// stalled upstream must not retain an account/concurrency slot indefinitely.
const clientDisconnectDrainFallbackTimeout = 5 * time.Second

// clientDisconnectDrainGuard keeps a detached upstream response bounded after
// the original HTTP request is canceled.  It is intentionally independent of
// detachUpstreamContext: the latter keeps the upstream request alive while the
// guard provides the finite post-disconnect drain window.
type clientDisconnectDrainGuard struct {
	stopOnce     sync.Once
	triggerOnce  sync.Once
	stop         chan struct{}
	clientGone   chan struct{}
	done         chan struct{}
	body         io.ReadCloser
	drainTimeout time.Duration
}

// clientDisconnectDrainTimeout resolves the post-disconnect drain budget. A
// configured positive stream interval may shorten the fallback; it never
// extends the fixed bound for disconnected clients.
func clientDisconnectDrainTimeout(cfg *config.Config) time.Duration {
	timeout := clientDisconnectDrainFallbackTimeout
	if cfg != nil && cfg.Gateway.StreamDataIntervalTimeout > 0 {
		configured := time.Duration(cfg.Gateway.StreamDataIntervalTimeout) * time.Second
		if configured < timeout {
			timeout = configured
		}
	}
	if timeout <= 0 {
		return time.Second
	}
	return timeout
}

func originalClientRequestContext(ctx context.Context, c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

// startClientDisconnectDrainGuard starts an idempotent watcher.  Trigger can
// be called by protocol handlers when a write fails before net/http propagates
// cancellation through the request context.
func startClientDisconnectDrainGuard(ctx context.Context, body io.ReadCloser, cfg *config.Config) *clientDisconnectDrainGuard {
	return startClientDisconnectDrainGuardWithTimeout(ctx, body, clientDisconnectDrainTimeout(cfg))
}

func startClientDisconnectDrainGuardWithTimeout(ctx context.Context, body io.ReadCloser, timeout time.Duration) *clientDisconnectDrainGuard {
	if timeout <= 0 {
		timeout = time.Second
	}
	guard := &clientDisconnectDrainGuard{
		stop:         make(chan struct{}),
		clientGone:   make(chan struct{}),
		done:         make(chan struct{}),
		body:         body,
		drainTimeout: timeout,
	}
	if body == nil {
		close(guard.done)
		return guard
	}

	go guard.run(ctx)
	return guard
}

func (g *clientDisconnectDrainGuard) run(ctx context.Context) {
	defer close(g.done)
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-g.stop:
		return
	case <-g.clientGone:
	case <-ctx.Done():
	}

	timer := time.NewTimer(g.drainTimeout)
	defer timer.Stop()
	select {
	case <-g.stop:
		return
	case <-timer.C:
		// Closing an HTTP response body is the portable way to interrupt a
		// blocked Scanner/ReadString/io.ReadAll on a detached transport.
		_ = g.body.Close()
	}
}

// ClientDisconnected starts the bounded drain window. It is safe to call from
// multiple protocol state-machine branches.
func (g *clientDisconnectDrainGuard) ClientDisconnected() {
	if g == nil {
		return
	}
	g.triggerOnce.Do(func() { close(g.clientGone) })
}

// Stop cancels the watcher and waits for it to finish. It is safe to call more
// than once and should be deferred by the owner of the response body.
func (g *clientDisconnectDrainGuard) Stop() {
	if g == nil {
		return
	}
	g.stopOnce.Do(func() { close(g.stop) })
	<-g.done
}
