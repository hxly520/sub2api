//go:build unit

package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"testing"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.WarnLevel)
	return zap.New(core), logs
}

func loggedFields(t *testing.T, logs *observer.ObservedLogs) map[string]any {
	t.Helper()
	entries := logs.All()
	require.Len(t, entries, 1)
	fields := map[string]any{}
	for _, f := range entries[0].Context {
		switch f.Key {
		case "body_len":
			fields[f.Key] = int(f.Integer)
		case "error":
			fields[f.Key] = f.Interface.(error).Error()
		default:
			fields[f.Key] = f.String
		}
	}
	return fields
}

func TestLogRequestBodyParseFailure_DerivesErrorWhenNil(t *testing.T) {
	log, logs := newObservedLogger(t)
	body := []byte(`{"model": bad}`)

	logRequestBodyParseFailure(log, body, nil)

	fields := loggedFields(t, logs)
	require.Equal(t, len(body), fields["body_len"])
	require.Contains(t, fields["error"], "invalid json")
	require.Contains(t, fields["error"], "offset=11")
}

func TestLogRequestBodyParseFailure_DoesNotLogPayload(t *testing.T) {
	log, logs := newObservedLogger(t)
	body := []byte("{\"prompt\":\"private customer prompt\",\"broken\":")

	logRequestBodyParseFailure(log, body, nil)

	fields := loggedFields(t, logs)
	require.Equal(t, len(body), fields["body_len"])
	digest := sha256.Sum256(body)
	require.Equal(t, hex.EncodeToString(digest[:]), fields["body_sha256"])
	require.NotContains(t, fields, "body_head")
	require.NotContains(t, fields, "body_tail")
	require.NotContains(t, fmt.Sprint(fields), "private customer prompt")
}

func TestLogRequestBodyParseFailure_LargeBodyLogsOnlyDigest(t *testing.T) {
	log, logs := newObservedLogger(t)
	body := make([]byte, 1<<20)
	copy(body, []byte("private customer prompt"))

	logRequestBodyParseFailure(log, body, nil)

	fields := loggedFields(t, logs)
	require.Equal(t, len(body), fields["body_len"])
	digest := sha256.Sum256(body)
	require.Equal(t, hex.EncodeToString(digest[:]), fields["body_sha256"])
	require.NotContains(t, fmt.Sprint(fields), "private customer prompt")
}

func TestLogRequestBodyParseFailure_BinaryBodyDoesNotLeak(t *testing.T) {
	log, logs := newObservedLogger(t)
	body := []byte("{\"model\":\x01\n\"x\"}")

	logRequestBodyParseFailure(log, body, nil)

	fields := loggedFields(t, logs)
	digest := sha256.Sum256(body)
	require.Equal(t, hex.EncodeToString(digest[:]), fields["body_sha256"])
	require.NotContains(t, fields, "body_head")
	require.NotContains(t, fields, "body_tail")
}

func TestLogRequestBodyParseFailure_NilLoggerNoPanic(t *testing.T) {
	require.NotPanics(t, func() {
		logRequestBodyParseFailure(nil, []byte(`{`), nil)
	})
}

func TestLogRequestBodyReadFailure_RecordsPayloadFreeTransportDiagnostics(t *testing.T) {
	log, logs := newObservedLogger(t)
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
	require.NoError(t, err)
	req.ContentLength = 4096
	req.Header.Set("Content-Encoding", "zstd")
	readErr := &pkghttputil.RequestBodyReadError{
		Stage:           pkghttputil.RequestBodyReadStageTransport,
		BytesRead:       2048,
		ContentLength:   4096,
		ContentEncoding: "zstd",
		Err:             io.ErrUnexpectedEOF,
	}

	logRequestBodyReadFailure(log, req, readErr)

	entries := logs.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, true, fields["upload_interrupted"])
	require.EqualValues(t, 2048, fields["body_bytes_read"])
	require.EqualValues(t, 4096, fields["declared_content_length"])
	require.Equal(t, "transport", fields["read_stage"])
	require.Equal(t, "zstd", fields["read_content_encoding"])
	require.NotContains(t, fmt.Sprint(fields), "model")
	require.Equal(t, "Request body upload was interrupted before completion", requestBodyReadFailureMessage(readErr))
}

func TestRequestBodyReadFailureMessage_DistinguishesDecodeFailure(t *testing.T) {
	err := &pkghttputil.RequestBodyReadError{
		Stage: pkghttputil.RequestBodyReadStageDecode,
		Err:   fmt.Errorf("decode zstd: corrupt frame"),
	}
	require.Equal(t, "Failed to decode compressed request body", requestBodyReadFailureMessage(err))
}
