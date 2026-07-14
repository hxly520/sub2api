package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

const samplePayload = `{"model":"gpt-5.5","input":"hi","stream":false}`

type bodyThenError struct {
	data []byte
	err  error
}

func (r *bodyThenError) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

func (r *bodyThenError) Close() error { return nil }

func newRequestWithBody(t *testing.T, body []byte, encoding string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	req.ContentLength = int64(len(body))
	return req
}

func TestReadRequestBodyWithPrealloc_PassesThroughIdentity(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesZstd(t *testing.T) {
	enc, _ := zstd.NewWriter(nil)
	compressed := enc.EncodeAll([]byte(samplePayload), nil)
	_ = enc.Close()

	req := newRequestWithBody(t, compressed, "zstd")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
	if req.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be cleared after decoding")
	}
	if req.ContentLength != int64(len(samplePayload)) {
		t.Fatalf("ContentLength not updated: %d", req.ContentLength)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesGzip(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(samplePayload)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	req := newRequestWithBody(t, buf.Bytes(), "gzip")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesDeflate(t *testing.T) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write([]byte(samplePayload)); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}

	req := newRequestWithBody(t, buf.Bytes(), "deflate")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_RejectsUnsupportedEncoding(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "br")
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for unsupported encoding, got nil")
	}
	if !strings.Contains(err.Error(), "br") {
		t.Fatalf("error should mention encoding, got %v", err)
	}
}

func TestReadRequestBodyWithPrealloc_RejectsCorruptZstd(t *testing.T) {
	req := newRequestWithBody(t, []byte("not actually zstd"), "zstd")
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for corrupt zstd body, got nil")
	}
}

func TestReadRequestBodyWithPrealloc_NilBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil body, got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_RespectsIdentityEncoding(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "identity")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadLenientJSONRequestBodyWithPrealloc_RecoversCompleteDeclaredBodyAfterTransportError(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Body = &bodyThenError{data: []byte(samplePayload), err: io.ErrUnexpectedEOF}
	req.ContentLength = int64(len(samplePayload))

	got, err := ReadLenientJSONRequestBodyWithPrealloc(req, 1<<20)
	if err != nil {
		t.Fatalf("expected complete JSON recovery, got %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadLenientJSONRequestBodyWithPrealloc_RecoversCompleteDeclaredCompressedFrameAfterTransportError(t *testing.T) {
	enc, _ := zstd.NewWriter(nil)
	compressed := enc.EncodeAll([]byte(samplePayload), nil)
	_ = enc.Close()

	req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Body = &bodyThenError{data: compressed, err: io.ErrUnexpectedEOF}
	req.ContentLength = int64(len(compressed))
	req.Header.Set("Content-Encoding", "zstd")

	got, err := ReadLenientJSONRequestBodyWithPrealloc(req, 1<<20)
	if err != nil {
		t.Fatalf("expected complete compressed JSON recovery, got %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
	if req.Header.Get("Content-Encoding") != "" {
		t.Fatal("Content-Encoding should be cleared after recovery")
	}
}

func TestReadLenientJSONRequestBodyWithPrealloc_RejectsValidJSONWhenDeclaredBodyIsIncomplete(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Body = &bodyThenError{data: []byte(samplePayload), err: io.ErrUnexpectedEOF}
	req.ContentLength = int64(len(samplePayload) + 10)

	_, err = ReadLenientJSONRequestBodyWithPrealloc(req, 1<<20)
	if err == nil {
		t.Fatal("expected an incomplete declared body to fail even when its prefix is valid JSON")
	}
}

func TestReadLenientJSONRequestBodyWithPrealloc_RejectsTruncatedJSONAfterInterruptedUpload(t *testing.T) {
	truncated := []byte(`{"model":"gpt-5.6-sol","input":[`)
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Body = &bodyThenError{data: truncated, err: io.ErrUnexpectedEOF}
	req.ContentLength = int64(len(truncated) + 100)

	_, err = ReadLenientJSONRequestBodyWithPrealloc(req, 1<<20)
	if err == nil {
		t.Fatal("expected interrupted truncated body to fail")
	}
	if !IsInterruptedRequestBodyError(err) {
		t.Fatalf("expected interrupted error classification, got %v", err)
	}
	var detail *RequestBodyReadError
	if !errors.As(err, &detail) {
		t.Fatalf("expected RequestBodyReadError, got %T", err)
	}
	if detail.BytesRead != int64(len(truncated)) {
		t.Fatalf("unexpected bytes read: %d", detail.BytesRead)
	}
}

func TestReadLenientJSONRequestBodyWithPrealloc_DoesNotRecoverMaxBytesError(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(samplePayload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Body = http.MaxBytesReader(nil, req.Body, int64(len(samplePayload)-1))

	_, err = ReadLenientJSONRequestBodyWithPrealloc(req, 1<<20)
	if err == nil {
		t.Fatal("expected max body error")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("expected MaxBytesError, got %T: %v", err, err)
	}
}

func TestIsInterruptedRequestBodyError_DoesNotMisclassifyDecodeFailure(t *testing.T) {
	err := &RequestBodyReadError{
		Stage: RequestBodyReadStageDecode,
		Err:   io.ErrUnexpectedEOF,
	}
	if IsInterruptedRequestBodyError(err) {
		t.Fatal("decode-stage unexpected EOF must not be classified as a transport interruption")
	}
}

func TestReadDecompressedRequestBodyRejectsTruncationAtLimit(t *testing.T) {
	t.Parallel()

	body, err := readDecompressedRequestBody(strings.NewReader(`{"ok":true}`), 4)
	if body != nil {
		t.Fatalf("expected no truncated body, got %q", body)
	}
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("expected MaxBytesError, got %T: %v", err, err)
	}
	if maxErr.Limit != 4 {
		t.Fatalf("MaxBytesError.Limit = %d, want 4", maxErr.Limit)
	}
}
