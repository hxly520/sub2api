package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// PrepareOpenAIImageClientResponseHeaders removes provider-controlled
// navigation and cache metadata before a protected image result is returned.
func PrepareOpenAIImageClientResponseHeaders(header http.Header) {
	if header == nil {
		return
	}
	for _, key := range []string{
		"Location",
		"Content-Location",
		"Refresh",
		"Link",
		"Set-Cookie",
		"ETag",
		"Last-Modified",
		"Expires",
	} {
		header.Del(key)
	}
	header.Set("Cache-Control", "no-store")
}

// PrepareOpenAIImageClientResponseBody replaces provider-hosted image URLs
// with encrypted edge URLs. Embedded image data is returned unchanged.
func (s *OpenAIGatewayService) PrepareOpenAIImageClientResponseBody(
	ctx context.Context,
	body []byte,
) ([]byte, error) {
	if len(body) == 0 || !json.Valid(body) {
		return body, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("invalid upstream image response")
	}

	state := openAIImageClientRewriteState{service: s, ctx: ctx}
	payload, err := state.rewrite(payload, "", "", nil)
	if err != nil {
		return nil, err
	}
	if !state.changed {
		return body, nil
	}

	rewrittenBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("image result delivery failed")
	}
	return rewrittenBody, nil
}

// PrepareOpenAIImageClientStreamLine applies the same URL policy to a single
// SSE line or a raw JSON fallback without buffering the generated media.
func (s *OpenAIGatewayService) PrepareOpenAIImageClientStreamLine(ctx context.Context, line []byte) ([]byte, error) {
	trimmedLine := bytes.TrimRight(line, "\r\n")
	if len(trimmedLine) == 0 {
		return line, nil
	}
	lineEnding := line[len(trimmedLine):]
	payload := trimmedLine
	isDataLine := false
	if data, ok := extractOpenAISSEDataLine(string(trimmedLine)); ok {
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			return line, nil
		}
		payload = []byte(data)
		isDataLine = true
	}
	if !json.Valid(payload) {
		if containsPotentialOpenAIImageExternalURL(string(payload)) {
			return nil, fmt.Errorf("image result delivery failed")
		}
		return line, nil
	}
	rewritten, err := s.PrepareOpenAIImageClientResponseBody(ctx, payload)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(rewritten, payload) {
		return line, nil
	}
	result := make([]byte, 0, len(rewritten)+len(lineEnding)+6)
	if isDataLine {
		result = append(result, "data: "...)
	}
	result = append(result, rewritten...)
	result = append(result, lineEnding...)
	return result, nil
}

func containsPotentialOpenAIImageExternalURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "http:") || strings.Contains(value, "https:") || strings.Contains(value, "://") || strings.HasPrefix(value, "//")
}

type openAIImageClientRewriteState struct {
	service *OpenAIGatewayService
	ctx     context.Context
	changed bool
}

func (s *openAIImageClientRewriteState) rewrite(value any, path, parentKey string, object map[string]any) (any, error) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			rewritten, err := s.rewrite(child, childPath, key, node)
			if err != nil {
				return nil, err
			}
			node[key] = rewritten
		}
		return node, nil
	case []any:
		for index, child := range node {
			rewritten, err := s.rewrite(child, path+"[]", parentKey, object)
			if err != nil {
				return nil, err
			}
			node[index] = rewritten
		}
		return node, nil
	case string:
		return s.rewriteString(node, path, parentKey, object)
	default:
		return value, nil
	}
}

func (s *openAIImageClientRewriteState) rewriteString(value, path, key string, object map[string]any) (string, error) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(trimmed), "data:image/") {
		return value, nil
	}
	isRemote := isOpenAIImageRemoteURL(trimmed)
	isResultURL := false
	if object != nil {
		objectPath := strings.TrimSuffix(path, "."+key)
		isResultURL = isOpenAIImageResultURLSlot(object, objectPath, key)
	} else {
		isResultURL = isOpenAIImageResultURLArrayPath(strings.TrimSuffix(path, "[]"))
	}
	if isResultURL && isRemote {
		if s.service == nil || s.service.cfg == nil || s.service.cfg.Gateway.VideoProxy.Mode != config.VideoProxyModeEdge {
			return "", fmt.Errorf("image edge proxy is unavailable")
		}
		edgeURL, err := s.service.openAIMediaEdgeProxyURL(s.ctx, "image", trimmed, nil, true)
		if err != nil {
			return "", fmt.Errorf("image result delivery failed")
		}
		s.changed = true
		return edgeURL, nil
	}
	if isRemote || strings.HasPrefix(trimmed, "//") || strings.Contains(trimmed, "://") {
		s.changed = true
		return "", nil
	}
	return value, nil
}

func isOpenAIImageResultURLSlot(object map[string]any, path, key string) bool {
	normalizedKey := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch normalizedKey {
	case "url", "image_url", "download_url", "result_url", "output_url", "media_url":
		return true
	}
	if normalizedKey == "result" || normalizedKey == "output" {
		if value, ok := object["type"].(string); ok && strings.Contains(strings.ToLower(value), "image") {
			return true
		}
		if _, ok := object["revised_prompt"]; ok {
			return true
		}
		return strings.Contains(path, "data[]") || strings.Contains(path, "output[]")
	}
	return false
}

func isOpenAIImageResultURLArrayPath(path string) bool {
	segment := path
	if index := strings.LastIndex(segment, "."); index >= 0 {
		segment = segment[index+1:]
	}
	switch strings.TrimSuffix(segment, "[]") {
	case "data", "images", "output", "results":
		return true
	default:
		return false
	}
}

func isOpenAIImageRemoteURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
