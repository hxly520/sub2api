package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"

	"github.com/klauspost/compress/zstd"
)

const (
	RequestBodyReadStageTransport = "transport"
	RequestBodyReadStageDecode    = "decode"
)

// RequestBodyReadError keeps payload-free diagnostics for request uploads.
// Callers can log the transfer stage and byte count without logging user data.
type RequestBodyReadError struct {
	Stage           string
	BytesRead       int64
	ContentLength   int64
	ContentEncoding string
	Err             error
}

func (e *RequestBodyReadError) Error() string {
	if e == nil || e.Err == nil {
		return "request body read failed"
	}
	return e.Err.Error()
}

func (e *RequestBodyReadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsInterruptedRequestBodyError(err error) bool {
	if err == nil {
		return false
	}
	var detail *RequestBodyReadError
	if errors.As(err, &detail) && detail.Stage != RequestBodyReadStageTransport {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"unexpected eof",
		"connection reset by peer",
		"broken pipe",
		"client disconnected",
		"context canceled",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

const (
	requestBodyReadInitCap    = 512
	requestBodyReadMaxInitCap = 1 << 20
	jsonUTF8BOMLen            = 3
	// maxDecompressedBodySize limits the decompressed request body to 64 MB
	// to prevent decompression bomb attacks.
	maxDecompressedBodySize = 64 << 20
)

// ReadRequestBodyWithPrealloc reads request body with preallocated buffer based
// on content length, transparently decoding any Content-Encoding the upstream
// client used to compress the body (zstd, gzip, deflate).
func ReadRequestBodyWithPrealloc(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}

	capHint := requestBodyReadInitCap
	if req.ContentLength > 0 {
		switch {
		case req.ContentLength < int64(requestBodyReadInitCap):
			capHint = requestBodyReadInitCap
		case req.ContentLength > int64(requestBodyReadMaxInitCap):
			capHint = requestBodyReadMaxInitCap
		default:
			capHint = int(req.ContentLength)
		}
	}

	buf := bytes.NewBuffer(make([]byte, 0, capHint))
	bytesRead, err := io.Copy(buf, req.Body)
	if err != nil {
		return buf.Bytes(), &RequestBodyReadError{
			Stage:           RequestBodyReadStageTransport,
			BytesRead:       bytesRead,
			ContentLength:   req.ContentLength,
			ContentEncoding: normalizedRequestContentEncoding(req),
			Err:             err,
		}
	}
	raw := buf.Bytes()

	enc := normalizedRequestContentEncoding(req)
	if enc == "" || enc == "identity" {
		return raw, nil
	}

	decoded, err := decompressRequestBody(enc, raw)
	if err != nil {
		return nil, &RequestBodyReadError{
			Stage:           RequestBodyReadStageDecode,
			BytesRead:       int64(len(raw)),
			ContentLength:   req.ContentLength,
			ContentEncoding: enc,
			Err:             fmt.Errorf("decode Content-Encoding %q: %w", enc, err),
		}
	}

	markRequestBodyDecoded(req, decoded)

	return decoded, nil
}

// ReadLenientJSONRequestBodyWithPrealloc reads a request body and normalizes
// JSON string control bytes before strict validation.
func ReadLenientJSONRequestBodyWithPrealloc(req *http.Request, maxNormalizedBytes int64) ([]byte, error) {
	body, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		if recovered, ok := recoverCompleteJSONRequestBody(req, body, err, maxNormalizedBytes); ok {
			return recovered, nil
		}
		return nil, err
	}
	return NormalizeLenientJSONRequestBody(body, maxNormalizedBytes)
}

func recoverCompleteJSONRequestBody(req *http.Request, raw []byte, readErr error, maxNormalizedBytes int64) ([]byte, bool) {
	if req == nil || len(raw) == 0 || readErr == nil || req.Context().Err() != nil {
		return nil, false
	}
	var maxErr *http.MaxBytesError
	if errors.As(readErr, &maxErr) {
		return nil, false
	}
	var detail *RequestBodyReadError
	if !errors.As(readErr, &detail) || detail.Stage != RequestBodyReadStageTransport || !IsInterruptedRequestBodyError(readErr) {
		return nil, false
	}
	// JSON validity alone cannot prove that an interrupted upload is complete:
	// a valid object may only be a prefix of the body the client intended to
	// send. Only recover when every declared transport byte was received.
	if detail.ContentLength <= 0 || detail.BytesRead != detail.ContentLength {
		return nil, false
	}

	decoded := raw
	enc := normalizedRequestContentEncoding(req)
	if enc != "" && enc != "identity" {
		var err error
		decoded, err = decompressRequestBody(enc, raw)
		if err != nil {
			return nil, false
		}
	}
	normalized, err := NormalizeLenientJSONRequestBody(decoded, maxNormalizedBytes)
	if err != nil || !json.Valid(normalized) {
		return nil, false
	}
	if enc != "" && enc != "identity" {
		markRequestBodyDecoded(req, normalized)
	}
	return normalized, true
}

func normalizedRequestContentEncoding(req *http.Request) string {
	if req == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
}

func markRequestBodyDecoded(req *http.Request, decoded []byte) {
	if req == nil {
		return
	}
	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(decoded))
}

func decompressRequestBody(encoding string, raw []byte) ([]byte, error) {
	switch encoding {
	case "zstd":
		dec, err := zstd.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer dec.Close()
		return readDecompressedRequestBody(dec, maxDecompressedBodySize)
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = gr.Close() }()
		return readDecompressedRequestBody(gr, maxDecompressedBodySize)
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		return readDecompressedRequestBody(zr, maxDecompressedBodySize)
	default:
		return nil, errors.New("unsupported Content-Encoding")
	}
}

func readDecompressedRequestBody(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, &http.MaxBytesError{Limit: limit}
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, &http.MaxBytesError{Limit: limit}
	}
	return body, nil
}

// NormalizeLenientJSONRequestBody escapes raw control bytes that broken
// OpenAI-compatible clients sometimes place inside JSON strings.
func NormalizeLenientJSONRequestBody(body []byte, maxNormalizedBytes int64) ([]byte, error) {
	if maxNormalizedBytes <= 0 {
		maxNormalizedBytes = maxDecompressedBodySize
	}

	body = trimUTF8BOM(body)
	if len(body) == 0 {
		return body, nil
	}
	if int64(len(body)) > maxNormalizedBytes {
		return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
	}

	var out []byte
	inString := false
	escaped := false
	for i, b := range body {
		if inString && isJSONControlByte(b) {
			if out == nil {
				capHint := len(body) + 6
				if int64(capHint) > maxNormalizedBytes {
					capHint = int(maxNormalizedBytes)
				}
				out = make([]byte, 0, capHint)
				out = append(out, body[:i]...)
			}
			if int64(len(out)+6) > maxNormalizedBytes {
				return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			out = appendJSONUnicodeEscape(out, b)
			escaped = false
			continue
		}

		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		}

		if out != nil {
			if int64(len(out)+1) > maxNormalizedBytes {
				return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			out = append(out, b)
		}
	}
	if out != nil {
		return out, nil
	}
	return body, nil
}

func trimUTF8BOM(body []byte) []byte {
	if len(body) >= jsonUTF8BOMLen && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		return body[jsonUTF8BOMLen:]
	}
	return body
}

func isJSONControlByte(b byte) bool {
	return b < 0x20 || b == 0x7f
}

func appendJSONUnicodeEscape(dst []byte, b byte) []byte {
	const hex = "0123456789abcdef"
	return append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0x0f])
}
