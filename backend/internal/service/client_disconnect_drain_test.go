package service

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

type blockingDrainReadCloser struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingDrainReadCloser() *blockingDrainReadCloser {
	return &blockingDrainReadCloser{closed: make(chan struct{})}
}

func (r *blockingDrainReadCloser) Read([]byte) (int, error) {
	<-r.closed
	return 0, io.ErrClosedPipe
}

func (r *blockingDrainReadCloser) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func TestClientDisconnectDrainGuardClosesBlockedBodyAfterBound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	body := newBlockingDrainReadCloser()
	guard := startClientDisconnectDrainGuardWithTimeout(ctx, body, 20*time.Millisecond)
	readDone := make(chan error, 1)
	go func() {
		_, err := body.Read(make([]byte, 1))
		readDone <- err
	}()

	cancel()
	select {
	case err := <-readDone:
		if err != io.ErrClosedPipe {
			t.Fatalf("blocked read returned %v, want io.ErrClosedPipe", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("drain guard did not close blocked body")
	}
	guard.Stop()
}

func TestClientDisconnectDrainGuardExplicitTriggerAndStopAreIdempotent(t *testing.T) {
	body := newBlockingDrainReadCloser()
	guard := startClientDisconnectDrainGuardWithTimeout(context.Background(), body, time.Second)
	guard.ClientDisconnected()
	guard.ClientDisconnected()
	guard.Stop()
	guard.Stop()
	select {
	case <-body.closed:
		t.Fatal("stopping guard should prevent a pending drain close")
	default:
	}
}
