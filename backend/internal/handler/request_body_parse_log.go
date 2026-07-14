package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// logRequestBodyParseFailure records the real reason a request body failed
// JSON parsing/validation. The client keeps receiving the generic
// "Failed to parse request body"; payload-free diagnostics land in the server
// log so operators can correlate failures without retaining user prompts.
//
// err may be nil for call sites that validate with gjson.ValidBytes directly;
// the diagnostic error is derived from the body in that case.
func logRequestBodyParseFailure(reqLog *zap.Logger, body []byte, err error) {
	if reqLog == nil {
		return
	}
	if err == nil {
		err = service.DescribeInvalidJSON(body)
	}

	digest := sha256.Sum256(body)
	fields := []zap.Field{
		zap.Error(err),
		zap.Int("body_len", len(body)),
		zap.String("body_sha256", hex.EncodeToString(digest[:])),
	}
	reqLog.Warn("parse request body failed", fields...)
}

func logRequestBodyReadFailure(reqLog *zap.Logger, req *http.Request, err error) {
	if reqLog == nil || err == nil {
		return
	}
	fields := []zap.Field{
		zap.Error(err),
		zap.Bool("upload_interrupted", pkghttputil.IsInterruptedRequestBodyError(err)),
	}
	if req != nil {
		fields = append(fields,
			zap.Int64("content_length", req.ContentLength),
			zap.String("content_encoding", strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))),
			zap.Strings("transfer_encoding", req.TransferEncoding),
		)
		if contextErr := req.Context().Err(); contextErr != nil {
			fields = append(fields, zap.String("request_context_error", contextErr.Error()))
		}
	}
	var detail *pkghttputil.RequestBodyReadError
	if errors.As(err, &detail) {
		fields = append(fields,
			zap.String("read_stage", detail.Stage),
			zap.Int64("body_bytes_read", detail.BytesRead),
			zap.Int64("declared_content_length", detail.ContentLength),
		)
		if detail.ContentEncoding != "" {
			fields = append(fields, zap.String("read_content_encoding", detail.ContentEncoding))
		}
	}
	reqLog.Warn("read request body failed", fields...)
}

func requestBodyReadFailureMessage(err error) string {
	if pkghttputil.IsInterruptedRequestBodyError(err) {
		return "Request body upload was interrupted before completion"
	}
	var detail *pkghttputil.RequestBodyReadError
	if errors.As(err, &detail) && detail.Stage == pkghttputil.RequestBodyReadStageDecode {
		return "Failed to decode compressed request body"
	}
	return "Failed to read request body"
}
