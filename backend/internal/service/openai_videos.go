package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIVideosEndpoint              = "/v1/videos"
	openAIVideosGenerationsEndpoint   = "/v1/videos/generations"
	openAIVideoGenerationsEndpoint    = "/v1/video/generations"
	openAIContentsGenerationTasksPath = "/contents/generations/tasks"
	openAIVideoMaxContentBytes        = int64(4 << 30)
	openAIVideoEdgeTokenAAD           = "sub2api-video-proxy-v1"
)

type openAIVideoEdgeTokenPayload struct {
	Version   int               `json:"v"`
	URL       string            `json:"u"`
	ExpiresAt int64             `json:"e"`
	Headers   map[string]string `json:"h,omitempty"`
	MediaType string            `json:"m,omitempty"`
}

type OpenAIVideoRequest struct {
	Endpoint                string
	UpstreamPath            string
	Method                  string
	ContentType             string
	Multipart               bool
	Model                   string
	ExplicitModel           bool
	Prompt                  string
	Size                    string
	Resolution              string
	DurationSeconds         int
	Stream                  bool
	GenerationRequest       bool
	ContentRequest          bool
	RequestID               string
	UpstreamRequestID       string
	ReferenceImageCount     int
	ReferenceVideoCount     int
	ReferenceAudioCount     int
	InputReferenceFileCount int
	ReferenceVideoBytes     int64
	HasFirstFrame           bool
	HasLastFrame            bool
	ReferenceMode           string
	AspectRatio             string
	UpstreamIdempotencyKey  string
	Body                    []byte
	bodyHash                string
}

type openAIVideoModelProfile int

const (
	openAIVideoModelUnknown openAIVideoModelProfile = iota
	openAIVideoModelGrok
	openAIVideoModelGrok15
	openAIVideoModelSeedanceStandard
	openAIVideoModelSeedancePerSecond
	openAIVideoModelOmniFast
	openAIVideoModelOmniV2V
	openAIVideoModelSora
	openAIVideoModelVeo
)

func (r *OpenAIVideoRequest) StickySessionSeed() string {
	if r == nil {
		return ""
	}
	parts := []string{
		"openai-video",
		strings.TrimSpace(r.Endpoint),
		strings.TrimSpace(r.Model),
		strings.TrimSpace(r.Prompt),
	}
	if r.bodyHash != "" {
		parts = append(parts, "body="+r.bodyHash)
	}
	return strings.Join(parts, "|")
}

func (r *OpenAIVideoRequest) BillingSizeTier() string {
	if r == nil {
		return NormalizeImageBillingTierOrDefault("")
	}
	if size := strings.TrimSpace(r.Size); size != "" {
		return NormalizeImageBillingTierOrDefault(size)
	}
	if resolution := strings.ToLower(strings.TrimSpace(r.Resolution)); resolution != "" {
		switch {
		case strings.Contains(resolution, "2160"), strings.Contains(resolution, "4k"):
			return ImageBillingSize4K
		case strings.Contains(resolution, "1080"), strings.Contains(resolution, "2k"):
			return ImageBillingSize2K
		case strings.Contains(resolution, "480"), strings.Contains(resolution, "720"), strings.Contains(resolution, "1k"):
			return ImageBillingSize1K
		}
	}
	return NormalizeImageBillingTierOrDefault("")
}

func (r *OpenAIVideoRequest) ModerationBody() []byte {
	if r == nil {
		return nil
	}
	prompt := strings.TrimSpace(r.Prompt)
	if prompt == "" {
		return nil
	}
	return []byte(fmt.Sprintf(`{"prompt":%q}`, prompt))
}

func (s *OpenAIGatewayService) ParseOpenAIVideoRequest(c *gin.Context, body []byte) (*OpenAIVideoRequest, error) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return nil, fmt.Errorf("missing request context")
	}
	req := normalizeOpenAIVideoEndpointPath(c.Request.Method, c.Request.URL.Path)
	if req == nil {
		return nil, fmt.Errorf("unsupported videos endpoint")
	}
	req.ContentType = strings.TrimSpace(c.GetHeader("Content-Type"))
	req.Body = body
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		req.bodyHash = hex.EncodeToString(sum[:8])
	}
	if req.GenerationRequest {
		if len(body) == 0 {
			return nil, fmt.Errorf("request body is empty")
		}
		if req.ContentType == "" {
			req.ContentType = "application/json"
		}
		if err := parseOpenAIVideoBody(req); err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.Model) == "" {
			return nil, fmt.Errorf("model is required")
		}
		if req.Stream {
			return nil, fmt.Errorf("streaming video generation is not supported")
		}
		normalizeOpenAIVideoDerivedFields(req)
		if err := validateOpenAIVideoModelRequest(req); err != nil {
			return nil, err
		}
		req.UpstreamPath = OpenAIVideoUpstreamEndpointForModel(req.Model, req.UpstreamPath)
	}
	return req, nil
}

func normalizeOpenAIVideoEndpointPath(method, path string) *OpenAIVideoRequest {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	// Accept one conventional trailing slash, but reject duplicate separators.
	// Collapsing // would let a visually similar path bypass the exact endpoint
	// grammar and can also turn an encoded slash into a different task ID.
	if len(trimmed) > 1 && strings.HasSuffix(trimmed, "/") {
		trimmed = strings.TrimSuffix(trimmed, "/")
	}
	if trimmed == "/" || strings.Contains(trimmed, "//") {
		return nil
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method != http.MethodGet && method != http.MethodPost {
		return nil
	}

	type endpointCandidate struct {
		paths    []string
		endpoint string
	}
	candidates := []endpointCandidate{
		{paths: []string{"/v1/contents/generations/tasks", "/contents/generations/tasks"}, endpoint: openAIContentsGenerationTasksPath},
		{paths: []string{"/v1/videos/generations", "/videos/generations"}, endpoint: openAIVideosGenerationsEndpoint},
		{paths: []string{"/v1/video/generations", "/video/generations"}, endpoint: openAIVideoGenerationsEndpoint},
		{paths: []string{"/v1/videos", "/videos"}, endpoint: openAIVideosEndpoint},
	}
	for _, candidate := range candidates {
		for _, basePath := range candidate.paths {
			if trimmed != basePath && !strings.HasPrefix(trimmed, basePath+"/") {
				continue
			}
			suffix := strings.TrimPrefix(trimmed, basePath)
			parts := splitOpenAIVideoSuffix(suffix)
			switch {
			case len(parts) == 0 && method == http.MethodPost:
				return &OpenAIVideoRequest{
					Endpoint:          candidate.endpoint,
					UpstreamPath:      candidate.endpoint,
					Method:            method,
					GenerationRequest: true,
				}
			case len(parts) == 1 && method == http.MethodGet:
				return &OpenAIVideoRequest{
					Endpoint:     candidate.endpoint,
					UpstreamPath: candidate.endpoint + "/" + url.PathEscape(parts[0]),
					Method:       method,
					RequestID:    parts[0],
				}
			case len(parts) == 2 && method == http.MethodGet && strings.EqualFold(parts[1], "content"):
				return &OpenAIVideoRequest{
					Endpoint:       candidate.endpoint,
					UpstreamPath:   candidate.endpoint + "/" + url.PathEscape(parts[0]) + "/content",
					Method:         method,
					ContentRequest: true,
					RequestID:      parts[0],
				}
			default:
				return nil
			}
		}
	}
	return nil
}

func splitOpenAIVideoSuffix(suffix string) []string {
	trimmed := strings.Trim(suffix, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return nil
		}
	}
	return parts
}

func (r *OpenAIVideoRequest) UseUpstreamTaskID(taskID string) error {
	return r.UseUpstreamTaskIDAtEndpoint(taskID, "")
}

func (r *OpenAIVideoRequest) UseUpstreamTaskIDAtEndpoint(taskID, storedEndpoint string) error {
	if r == nil {
		return fmt.Errorf("video request is nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("upstream video task id is missing")
	}
	r.UpstreamRequestID = taskID
	endpoint := normalizeOpenAIVideoTaskEndpoint(storedEndpoint)
	if endpoint == "" {
		endpoint = OpenAIVideoUpstreamEndpointForModel(r.Model, r.Endpoint)
	}
	if endpoint == "" {
		return fmt.Errorf("upstream video task endpoint is unavailable")
	}
	r.UpstreamPath = strings.TrimRight(endpoint, "/") + "/" + url.PathEscape(taskID)
	if r.ContentRequest {
		r.UpstreamPath += "/content"
	}
	return nil
}

func normalizeOpenAIVideoTaskEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	switch endpoint {
	case openAIVideosEndpoint, openAIVideosGenerationsEndpoint, openAIVideoGenerationsEndpoint, openAIContentsGenerationTasksPath:
		return endpoint
	default:
		return ""
	}
}

func parseOpenAIVideoBody(req *OpenAIVideoRequest) error {
	mediaType, _, err := mime.ParseMediaType(req.ContentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		req.Multipart = true
		return parseOpenAIVideoMultipartBody(req)
	}
	if !gjson.ValidBytes(req.Body) {
		return fmt.Errorf("failed to parse request body")
	}
	req.Model = strings.TrimSpace(gjson.GetBytes(req.Body, "model").String())
	req.ExplicitModel = req.Model != ""
	req.Prompt = strings.TrimSpace(gjson.GetBytes(req.Body, "prompt").String())
	if req.Prompt == "" {
		req.Prompt = strings.TrimSpace(gjson.GetBytes(req.Body, "content.#(type==\"text\").text").String())
	}
	req.Size = strings.TrimSpace(gjson.GetBytes(req.Body, "size").String())
	req.AspectRatio = strings.TrimSpace(gjson.GetBytes(req.Body, "aspect_ratio").String())
	if req.AspectRatio == "" {
		req.AspectRatio = strings.TrimSpace(gjson.GetBytes(req.Body, "ratio").String())
	}
	req.HasFirstFrame = openAIVideoJSONReferenceExists(req.Body, []string{"first_image_url", "first_frame", "first_frame_url"})
	req.HasLastFrame = openAIVideoJSONReferenceExists(req.Body, []string{"last_image_url", "last_frame", "last_frame_url"})
	req.ReferenceImageCount = countOpenAIVideoJSONReferences(req.Body, []string{"image", "image_url", "image_urls", "images", "input_reference", "reference_image_urls", "reference_images"})
	if req.HasFirstFrame {
		req.ReferenceImageCount++
	}
	if req.HasLastFrame {
		req.ReferenceImageCount++
	}
	req.ReferenceVideoCount = countOpenAIVideoJSONReferences(req.Body, []string{"video", "video_url", "video_urls", "videos", "reference_videos", "input_video"})
	req.ReferenceAudioCount = countOpenAIVideoJSONReferences(req.Body, []string{"audio", "audio_url", "audio_urls", "audios", "reference_audios", "input_audio"})
	req.ReferenceMode = strings.TrimSpace(gjson.GetBytes(req.Body, "reference_mode").String())
	req.Resolution = strings.TrimSpace(gjson.GetBytes(req.Body, "resolution_name").String())
	if req.Resolution == "" {
		req.Resolution = strings.TrimSpace(gjson.GetBytes(req.Body, "resolution").String())
	}
	req.DurationSeconds = extractOpenAIVideoDurationSeconds(req.Body)
	if stream := gjson.GetBytes(req.Body, "stream"); stream.Exists() {
		req.Stream = stream.Bool()
	}
	return nil
}

func parseOpenAIVideoMultipartBody(req *OpenAIVideoRequest) error {
	_, params, err := mime.ParseMediaType(req.ContentType)
	if err != nil {
		return fmt.Errorf("parse multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return fmt.Errorf("multipart boundary is required")
	}
	reader := multipart.NewReader(bytes.NewReader(req.Body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read multipart body: %w", err)
		}
		name := strings.TrimSpace(part.FormName())
		lowerName := strings.TrimSuffix(strings.ToLower(name), "[]")
		fileName := strings.TrimSpace(part.FileName())
		if fileName != "" {
			switch {
			case strings.Contains(lowerName, "video"):
				req.ReferenceVideoCount++
				read, copyErr := io.Copy(io.Discard, io.LimitReader(part, (5<<20)+1))
				if copyErr != nil {
					_ = part.Close()
					return fmt.Errorf("read multipart video: %w", copyErr)
				}
				if read > req.ReferenceVideoBytes {
					req.ReferenceVideoBytes = read
				}
			case strings.Contains(lowerName, "audio"):
				req.ReferenceAudioCount++
			case lowerName == "first_frame" || lowerName == "first_image":
				req.HasFirstFrame = true
				req.ReferenceImageCount++
			case lowerName == "last_frame" || lowerName == "last_image":
				req.HasLastFrame = true
				req.ReferenceImageCount++
			case lowerName == "input_reference":
				req.InputReferenceFileCount++
				req.ReferenceImageCount++
			case strings.Contains(lowerName, "image") || strings.Contains(lowerName, "reference"):
				req.ReferenceImageCount++
			}
			_ = part.Close()
			continue
		}
		valueBytes, readErr := io.ReadAll(io.LimitReader(part, 1<<20))
		_ = part.Close()
		if readErr != nil {
			return fmt.Errorf("read multipart field: %w", readErr)
		}
		value := strings.TrimSpace(string(valueBytes))
		switch lowerName {
		case "model":
			req.Model = value
			req.ExplicitModel = value != ""
		case "prompt":
			req.Prompt = value
		case "size":
			req.Size = value
		case "ratio", "aspect_ratio":
			req.AspectRatio = value
		case "first_image_url", "first_frame", "first_frame_url":
			if value != "" {
				req.HasFirstFrame = true
				req.ReferenceImageCount++
			}
		case "last_image_url", "last_frame", "last_frame_url":
			if value != "" {
				req.HasLastFrame = true
				req.ReferenceImageCount++
			}
		case "image", "image_url", "image_urls", "images", "input_reference", "reference_image_urls", "reference_images":
			req.ReferenceImageCount += countOpenAIVideoFieldValues(value)
		case "video", "video_url", "video_urls", "videos", "input_video", "reference_videos":
			req.ReferenceVideoCount += countOpenAIVideoFieldValues(value)
		case "audio":
			if !strings.EqualFold(value, "true") && !strings.EqualFold(value, "false") {
				req.ReferenceAudioCount += countOpenAIVideoFieldValues(value)
			}
		case "audio_url", "audio_urls", "audios", "input_audio", "reference_audios":
			req.ReferenceAudioCount += countOpenAIVideoFieldValues(value)
		case "reference_mode":
			req.ReferenceMode = value
		case "resolution", "resolution_name":
			req.Resolution = value
		case "duration", "seconds", "video_duration", "sora2_duration":
			req.DurationSeconds = normalizeOpenAIVideoDurationSeconds(value)
		case "stream":
			req.Stream = strings.EqualFold(value, "true")
		}
	}
	return nil
}

func countOpenAIVideoJSONReferences(body []byte, paths []string) int {
	count := 0
	for _, path := range paths {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			continue
		}
		if value.IsBool() {
			continue
		}
		if value.IsArray() {
			count += len(value.Array())
			continue
		}
		if value.IsObject() || strings.TrimSpace(value.String()) != "" {
			count++
		}
	}
	return count
}

func openAIVideoJSONReferenceExists(body []byte, paths []string) bool {
	return countOpenAIVideoJSONReferences(body, paths) > 0
}

func countOpenAIVideoFieldValues(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if gjson.Valid(value) {
		parsed := gjson.Parse(value)
		if parsed.IsArray() {
			return len(parsed.Array())
		}
	}
	return 1
}

func openAIVideoFrameCount(req *OpenAIVideoRequest) int {
	if req == nil {
		return 0
	}
	count := 0
	if req.HasFirstFrame {
		count++
	}
	if req.HasLastFrame {
		count++
	}
	return count
}

func seedanceFixedResolutionFromModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, candidate := range []string{"2160p", "4k", "1080p", "720p", "480p"} {
		if strings.Contains(model, "-"+candidate) || strings.HasSuffix(model, candidate) {
			if candidate == "2160p" {
				return "4k"
			}
			return candidate
		}
	}
	return ""
}

// classifyOpenAIVideoModel groups the public model names exposed by the relay's
// model-api-doc-v1 video catalog. Family matching also covers administrator
// aliases such as cy-gv1-grok-video without coupling the gateway to one prefix.
func classifyOpenAIVideoModel(model string) openAIVideoModelProfile {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(model, "grok") && strings.Contains(model, "video") && strings.Contains(model, "1.5"):
		return openAIVideoModelGrok15
	case strings.Contains(model, "grok") && strings.Contains(model, "video"):
		return openAIVideoModelGrok
	case strings.Contains(model, "seedance") && seedanceFixedResolutionFromModel(model) != "":
		return openAIVideoModelSeedancePerSecond
	case strings.Contains(model, "seedance"):
		return openAIVideoModelSeedanceStandard
	case strings.Contains(model, "omni-v2v"):
		return openAIVideoModelOmniV2V
	case strings.Contains(model, "omni-fast"):
		return openAIVideoModelOmniFast
	case strings.Contains(model, "sora"):
		return openAIVideoModelSora
	case strings.Contains(model, "veo"):
		return openAIVideoModelVeo
	default:
		return openAIVideoModelUnknown
	}
}

func openAIVideoModelHasToken(model, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, token := range strings.FieldsFunc(strings.ToLower(model), func(r rune) bool {
		return r == '-' || r == '_' || r == ':' || r == '.' || r == '/'
	}) {
		if token == target {
			return true
		}
	}
	return false
}

func normalizeOpenAIVideoDerivedFields(req *OpenAIVideoRequest) {
	if req == nil || strings.TrimSpace(req.Resolution) != "" {
		return
	}
	switch classifyOpenAIVideoModel(req.Model) {
	case openAIVideoModelSeedancePerSecond:
		req.Resolution = seedanceFixedResolutionFromModel(req.Model)
	case openAIVideoModelOmniFast, openAIVideoModelOmniV2V:
		req.Resolution = VideoBillingResolution720P
	}
}

// OpenAIVideoUpstreamEndpointForModel keeps all supported client path aliases
// while routing the relay video catalog through the unified task endpoint.
// It never returns a vendor host, only a path below the configured base URL.
func OpenAIVideoUpstreamEndpointForModel(model, fallback string) string {
	lowerModel := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(lowerModel, "grok") && strings.Contains(lowerModel, "video"), strings.Contains(lowerModel, "seedance"), strings.Contains(lowerModel, "omni-"), strings.Contains(lowerModel, "sora"), strings.Contains(lowerModel, "veo"):
		return openAIVideosEndpoint
	default:
		return strings.TrimSpace(fallback)
	}
}

// PrepareOpenAIVideoRequestForUpstream applies the final account model mapping
// before validating the provider-specific contract and selecting its endpoint.
func PrepareOpenAIVideoRequestForUpstream(req *OpenAIVideoRequest, upstreamModel string) (*OpenAIVideoRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("video request is nil")
	}
	prepared := *req
	if model := strings.TrimSpace(upstreamModel); model != "" {
		prepared.Model = model
	}
	if !prepared.GenerationRequest {
		return &prepared, nil
	}
	normalizeOpenAIVideoDerivedFields(&prepared)
	if err := validateOpenAIVideoModelRequest(&prepared); err != nil {
		return nil, err
	}
	prepared.UpstreamPath = OpenAIVideoUpstreamEndpointForModel(prepared.Model, prepared.UpstreamPath)
	if strings.TrimSpace(prepared.UpstreamPath) == "" {
		return nil, fmt.Errorf("video upstream endpoint is unavailable")
	}
	return &prepared, nil
}

func validateOpenAIVideoModelRequest(req *OpenAIVideoRequest) error {
	if req == nil {
		return fmt.Errorf("video request is nil")
	}
	model := strings.ToLower(strings.TrimSpace(req.Model))
	profile := classifyOpenAIVideoModel(model)
	resolution := strings.ToLower(strings.TrimSpace(req.Resolution))
	ratio := strings.ToLower(strings.TrimSpace(req.AspectRatio))
	referenceMode := strings.ToLower(strings.TrimSpace(req.ReferenceMode))
	duration := req.DurationSeconds
	frameCount := openAIVideoFrameCount(req)
	nonFrameImageCount := req.ReferenceImageCount - frameCount
	if nonFrameImageCount < 0 {
		nonFrameImageCount = 0
	}

	if profile != openAIVideoModelUnknown && strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt is required for the selected video model")
	}
	if (profile == openAIVideoModelGrok || profile == openAIVideoModelGrok15) && utf8.RuneCountInString(req.Prompt) > 4096 {
		return fmt.Errorf("grok video prompt must not exceed 4096 characters")
	}
	if (profile == openAIVideoModelSeedanceStandard || profile == openAIVideoModelSeedancePerSecond) && utf8.RuneCountInString(req.Prompt) > 5000 {
		return fmt.Errorf("seedance video prompt must not exceed 5000 characters")
	}

	if profile == openAIVideoModelGrok || profile == openAIVideoModelGrok15 {
		if duration > 0 && !containsOpenAIVideoInt([]int{4, 6, 8, 10, 12, 15}, duration) {
			return fmt.Errorf("grok video duration must be one of 4, 6, 8, 10, 12, or 15 seconds")
		}
		if req.ReferenceImageCount > 7 {
			return fmt.Errorf("grok video supports at most seven reference images")
		}
		if resolution != "" && resolution != "480p" && resolution != "720p" {
			return fmt.Errorf("grok video resolution must be 480p or 720p")
		}
		if req.ReferenceVideoCount > 1 {
			return fmt.Errorf("grok video supports at most one video reference")
		}
		if req.ReferenceAudioCount > 0 {
			return fmt.Errorf("grok video does not accept audio references")
		}
		if frameCount > 0 {
			return fmt.Errorf("grok video does not accept first or last frame fields")
		}
	}
	switch profile {
	case openAIVideoModelGrok15:
		if req.ReferenceImageCount != 1 {
			return fmt.Errorf("grok video 1.5 requires exactly one reference image")
		}
		if req.ReferenceVideoCount != 0 {
			return fmt.Errorf("grok video 1.5 does not accept a video reference")
		}
		if ratio != "" && ratio != "16:9" && ratio != "9:16" {
			return fmt.Errorf("grok video 1.5 aspect_ratio must be 16:9 or 9:16")
		}
	case openAIVideoModelGrok:
		if ratio != "" && !containsOpenAIVideoString([]string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"}, ratio) {
			return fmt.Errorf("grok video aspect_ratio is unsupported")
		}
	}

	if profile == openAIVideoModelSeedanceStandard || profile == openAIVideoModelSeedancePerSecond {
		if duration > 0 && (duration < 4 || duration > 15) {
			return fmt.Errorf("seedance video duration must be between 4 and 15 seconds")
		}
		if ratio != "" && !containsOpenAIVideoString([]string{"16:9", "9:16", "1:1", "21:9", "3:4", "4:3"}, ratio) {
			return fmt.Errorf("seedance video aspect_ratio is unsupported")
		}
		fixedResolution := seedanceFixedResolutionFromModel(model)
		if fixedResolution == "" {
			if resolution != "" && resolution != "480p" && resolution != "720p" {
				return fmt.Errorf("seedance standard video resolution must be 480p or 720p")
			}
			if req.ReferenceImageCount > 4 || req.ReferenceVideoCount > 3 || req.ReferenceAudioCount > 1 {
				return fmt.Errorf("seedance standard video supports at most 4 images, 3 videos, and 1 audio reference")
			}
			if frameCount > 0 && (nonFrameImageCount > 0 || req.ReferenceVideoCount > 0 || req.ReferenceAudioCount > 0) {
				return fmt.Errorf("seedance first/last frames cannot be combined with multimodal references")
			}
			if frameCount == 0 && (req.ReferenceVideoCount > 0 || req.ReferenceAudioCount > 0) && nonFrameImageCount == 0 {
				return fmt.Errorf("seedance video or audio references require at least one image reference")
			}
		} else {
			if duration <= 0 {
				return fmt.Errorf("seedance per-second video duration is required")
			}
			if resolution != "" && resolution != fixedResolution && (resolution != "2160p" || fixedResolution != "4k") {
				return fmt.Errorf("seedance resolution is fixed by the selected model")
			}
			if req.ReferenceImageCount > 9 || req.ReferenceVideoCount > 3 || req.ReferenceAudioCount > 3 {
				return fmt.Errorf("seedance per-second video supports at most 9 images, 3 videos, and 3 audio references")
			}
			if frameCount > 0 && (nonFrameImageCount > 0 || req.ReferenceVideoCount > 0 || req.ReferenceAudioCount > 0) {
				return fmt.Errorf("seedance first/last frames cannot be combined with multimodal references")
			}
			if frameCount == 0 && (req.ReferenceVideoCount > 0 || req.ReferenceAudioCount > 0) && nonFrameImageCount == 0 {
				return fmt.Errorf("seedance video or audio references require at least one image reference")
			}
		}
		if req.HasFirstFrame != req.HasLastFrame {
			return fmt.Errorf("seedance first and last frames must be provided together")
		}
	}

	if profile == openAIVideoModelOmniFast {
		if duration > 0 && duration != 10 {
			return fmt.Errorf("omni fast video duration must be 10 seconds")
		}
		if resolution != "" && resolution != "720p" {
			return fmt.Errorf("omni fast video resolution must be 720p")
		}
		if ratio != "" && ratio != "16:9" && ratio != "9:16" {
			return fmt.Errorf("omni fast aspect_ratio must be 16:9 or 9:16")
		}
		if req.ReferenceImageCount > 5 {
			return fmt.Errorf("omni video supports at most five reference images")
		}
		if nonFrameImageCount > 1 && (!req.Multipart || req.InputReferenceFileCount != nonFrameImageCount) {
			return fmt.Errorf("omni multi-image video requires multipart input_reference files")
		}
		if req.ReferenceVideoCount > 0 || req.ReferenceAudioCount > 0 {
			return fmt.Errorf("omni fast does not accept video or audio references")
		}
	}

	if profile == openAIVideoModelOmniV2V {
		if req.ReferenceVideoCount != 1 {
			return fmt.Errorf("omni v2v requires exactly one source video")
		}
		if req.ReferenceVideoBytes > 5<<20 {
			return fmt.Errorf("omni v2v source video must not exceed 5MB")
		}
		if duration > 0 && duration != 10 {
			return fmt.Errorf("omni v2v duration must be 10 seconds")
		}
		if resolution != "" && resolution != "720p" {
			return fmt.Errorf("omni v2v output resolution must be 720p")
		}
		if ratio != "" && ratio != "16:9" && ratio != "9:16" {
			return fmt.Errorf("omni v2v aspect_ratio must be 16:9 or 9:16")
		}
		if req.ReferenceImageCount > 0 || req.ReferenceAudioCount > 0 {
			return fmt.Errorf("omni v2v only accepts a source video reference")
		}
	}

	if profile == openAIVideoModelSora {
		if duration > 0 && duration != 4 && duration != 8 && duration != 12 {
			return fmt.Errorf("sora video duration must be 4, 8, or 12 seconds")
		}
		if ratio != "" && ratio != "16:9" && ratio != "9:16" {
			return fmt.Errorf("sora video aspect_ratio must be 16:9 or 9:16")
		}
		size := strings.ToLower(strings.TrimSpace(req.Size))
		if size != "" && !containsOpenAIVideoString([]string{"1280x720", "720x1280", "1024x1024"}, size) {
			return fmt.Errorf("sora video size is unsupported")
		}
		if utf8.RuneCountInString(req.Prompt) > 1200 {
			return fmt.Errorf("sora video prompt must not exceed 1200 characters")
		}
		if req.ReferenceImageCount > 1 {
			return fmt.Errorf("sora video supports at most one frame reference image")
		}
		if req.ReferenceVideoCount > 0 || req.ReferenceAudioCount > 0 {
			return fmt.Errorf("sora video does not accept video or audio references")
		}
		if referenceMode != "" && referenceMode != "frame" {
			return fmt.Errorf("sora video reference_mode must be frame")
		}
	}

	if profile == openAIVideoModelVeo {
		if duration > 0 && !containsOpenAIVideoInt([]int{4, 6, 8}, duration) {
			return fmt.Errorf("veo video duration must be 4, 6, or 8 seconds")
		}
		if ratio != "" && ratio != "16:9" && ratio != "9:16" {
			return fmt.Errorf("veo video aspect_ratio must be 16:9 or 9:16")
		}
		if resolution != "" && resolution != "720p" && resolution != "1080p" {
			return fmt.Errorf("veo video resolution must be 720p or 1080p")
		}
		if utf8.RuneCountInString(req.Prompt) > 1200 {
			return fmt.Errorf("veo video prompt must not exceed 1200 characters")
		}
		isReferenceModel := strings.Contains(model, "veo") && openAIVideoModelHasToken(model, "ref")
		maxImages := 2
		expectedReferenceMode := "frame"
		if isReferenceModel {
			maxImages = 3
			expectedReferenceMode = "image"
		}
		if req.ReferenceImageCount > maxImages {
			return fmt.Errorf("veo video supports at most %d reference images", maxImages)
		}
		if req.ReferenceVideoCount > 0 || req.ReferenceAudioCount > 0 {
			return fmt.Errorf("veo video does not accept video or audio references")
		}
		if frameCount > 0 {
			return fmt.Errorf("veo video does not accept first or last frame fields")
		}
		if referenceMode != "" && referenceMode != expectedReferenceMode {
			return fmt.Errorf("veo video reference_mode must be %s", expectedReferenceMode)
		}
	}
	return nil
}

func containsOpenAIVideoInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsOpenAIVideoString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) ForwardVideo(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIVideoRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed video request is required")
	}
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	if account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("videos endpoint requires an OpenAI API key account")
	}
	if parsed.Stream {
		return nil, fmt.Errorf("streaming video generation is not supported")
	}

	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	upstreamModel := account.GetMappedModel(requestModel)
	var err error
	effectiveParsed := parsed
	if parsed.GenerationRequest {
		effectiveParsed, err = PrepareOpenAIVideoRequestForUpstream(parsed, upstreamModel)
		if err != nil {
			return nil, err
		}
	}

	forwardBody := parsed.Body
	forwardContentType := parsed.ContentType
	if parsed.GenerationRequest && strings.TrimSpace(upstreamModel) != "" {
		forwardBody, forwardContentType, err = rewriteOpenAIVideoRequest(parsed.Body, parsed.ContentType, upstreamModel, effectiveParsed)
		if err != nil {
			return nil, err
		}
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, parsed.ContentRequest)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIVideoRequest(upstreamCtx, c, account, effectiveParsed, forwardBody, forwardContentType, token)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:    account.Platform,
			AccountID:   account.ID,
			AccountName: account.Name,
			Kind:        "request_error",
			Message:     safeErr,
			UpstreamURL: safeUpstreamURL(upstreamReq.URL.String()),
		})
		return nil, fmt.Errorf("video upstream request failed")
	}
	if resp == nil || resp.Body == nil {
		setOpsUpstreamError(c, 0, "video upstream response is unavailable", "")
		return nil, fmt.Errorf("video upstream response is unavailable")
	}
	if resp.StatusCode >= http.StatusBadRequest {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if parsed.GenerationRequest && s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			s.handleFailoverSideEffects(upstreamCtx, resp, account, respBody, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:              resp.StatusCode,
				MediaOutcomeKnownFailed: IsDefinitiveMediaGenerationFailure(resp.StatusCode, respBody),
				ResponseBody:            respBody,
				RetryableOnSameAccount:  account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleOpenAIVideoErrorResponse(upstreamCtx, resp, c, account, upstreamModel)
	}
	defer func() { _ = resp.Body.Close() }()

	if effectiveParsed.ContentRequest {
		if err := s.streamOpenAIVideoContentResponse(resp, c); err != nil {
			return nil, err
		}
		return &OpenAIForwardResult{
			RequestID:      resp.Header.Get("x-request-id"),
			Model:          requestModel,
			UpstreamModel:  upstreamModel,
			Duration:       time.Since(startTime),
			ResponseStatus: resp.StatusCode,
			MediaType:      "video",
		}, nil
	}

	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("video upstream returned an invalid response")
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	responseID := extractOpenAIVideoTaskID(body)
	videoStatus := extractOpenAIVideoStatus(body)
	if videoStatus == "" && openAIVideoHasResultURL(body) {
		videoStatus = MediaGenerationStatusCompleted
	}
	if responseID == "" && effectiveParsed.GenerationRequest && IsMediaGenerationSuccessStatus(videoStatus) {
		responseID = localOpenAIVideoSynchronousTaskID(resp.Header.Get("x-request-id"), body)
	}
	if effectiveParsed.GenerationRequest && strings.TrimSpace(responseID) == "" {
		return nil, fmt.Errorf("video upstream response did not include a task id")
	}
	durationSeconds := effectiveParsed.DurationSeconds
	if durationSeconds <= 0 {
		durationSeconds = extractOpenAIVideoDurationSeconds(body)
	}
	contentType := "application/json"

	return &OpenAIForwardResult{
		RequestID:            resp.Header.Get("x-request-id"),
		ResponseID:           responseID,
		Usage:                usage,
		Model:                requestModel,
		BillingModel:         upstreamModel,
		UpstreamModel:        upstreamModel,
		Stream:               false,
		Duration:             time.Since(startTime),
		ImageCount:           0,
		VideoCount:           boolToMediaCount(effectiveParsed.GenerationRequest),
		ImageSize:            effectiveParsed.BillingSizeTier(),
		VideoResolution:      effectiveParsed.Resolution,
		VideoDurationSeconds: effectiveParsed.DurationSeconds,
		MediaDurationSeconds: durationSeconds,
		MediaType:            "video",
		ResponseStatus:       resp.StatusCode,
		ResponseBody:         body,
		ResponseContentType:  contentType,
		MediaResultURL:       OpenAIVideoResultURLFromBody(body),
		VideoStatus:          videoStatus,
	}, nil
}

func (s *OpenAIGatewayService) buildOpenAIVideoRequest(ctx context.Context, c *gin.Context, account *Account, parsed *OpenAIVideoRequest, body []byte, contentType, token string) (*http.Request, error) {
	// Video generation is intentionally relay-only. Unlike general OpenAI API-key
	// traffic, never fall back to the official OpenAI host when the account has
	// no explicit upstream configured.
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		return nil, fmt.Errorf("video account base_url is required")
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	targetURL := buildOpenAIEndpointURL(validatedURL, parsed.UpstreamPath)

	var reader io.Reader
	if parsed.Method != http.MethodGet && len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, parsed.Method, targetURL, reader)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	if parsed.ContentRequest {
		req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(req.Context()))
	}
	applyOpenAICompatibleAPIKeyAuth(req, account, token)
	for key, values := range c.Request.Header {
		if !openaiPassthroughAllowedHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if parsed.ContentRequest {
		for _, header := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
			if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
				req.Header.Set(header, value)
			}
		}
	}
	if parsed.GenerationRequest {
		idempotencyKey := strings.TrimSpace(parsed.UpstreamIdempotencyKey)
		if idempotencyKey == "" {
			idempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		}
		if idempotencyKey == "" {
			idempotencyKey = strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}
	}
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	if parsed.Method != http.MethodGet && strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func rewriteOpenAIVideoRequest(body []byte, contentType string, model string, parsed *OpenAIVideoRequest) ([]byte, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return body, contentType, nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		return rewriteOpenAIImagesMultipartModel(body, contentType, model)
	}
	rewritten, err := sjson.SetBytes(body, "model", model)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite video request model: %w", err)
	}
	if parsed != nil && classifyOpenAIVideoModel(model) == openAIVideoModelGrok && parsed.ReferenceImageCount > 1 && parsed.DurationSeconds > 10 {
		rewritten, err = sjson.SetBytes(rewritten, "seconds", 10)
		if err != nil {
			return nil, "", fmt.Errorf("rewrite grok video seconds: %w", err)
		}
		if gjson.GetBytes(rewritten, "duration").Exists() {
			rewritten, err = sjson.SetBytes(rewritten, "duration", 10)
			if err != nil {
				return nil, "", fmt.Errorf("rewrite grok video duration: %w", err)
			}
		}
	}
	return rewritten, contentType, nil
}

func (s *OpenAIGatewayService) streamOpenAIVideoContentResponse(resp *http.Response, c *gin.Context) error {
	if resp == nil || resp.Body == nil || c == nil || c.Writer == nil {
		return fmt.Errorf("video content response is unavailable")
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return fmt.Errorf("video content redirect is unavailable")
	}
	if resp.StatusCode != http.StatusNotModified && (resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices) {
		return fmt.Errorf("video content is unavailable")
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusNotModified {
		if !isAllowedOpenAIVideoContentType(contentType) {
			return fmt.Errorf("video content type is unavailable")
		}
	}
	if contentLength := strings.TrimSpace(resp.Header.Get("Content-Length")); contentLength != "" {
		parsedLength, err := strconv.ParseInt(contentLength, 10, 64)
		if err != nil || parsedLength < 0 || parsedLength > openAIVideoMaxContentBytes {
			return fmt.Errorf("video content is too large")
		}
	}
	for _, header := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Last-Modified",
		"Cache-Control",
	} {
		if value := strings.TrimSpace(resp.Header.Get(header)); value != "" {
			c.Header(header, value)
		}
	}
	c.Status(resp.StatusCode)
	if resp.StatusCode == http.StatusNotModified {
		return nil
	}
	written, err := io.Copy(c.Writer, io.LimitReader(resp.Body, openAIVideoMaxContentBytes+1))
	if err != nil {
		return err
	}
	if written > openAIVideoMaxContentBytes {
		return fmt.Errorf("video content is too large")
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func isAllowedOpenAIVideoContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return strings.HasPrefix(mediaType, "video/") ||
		mediaType == "application/octet-stream" ||
		mediaType == "binary/octet-stream" ||
		mediaType == "application/mp4"
}

func (s *OpenAIGatewayService) StreamOpenAIVideoTaskContent(ctx context.Context, c *gin.Context, task *MediaGenerationTask) (bool, error) {
	if task == nil {
		return false, nil
	}
	mediaURL := strings.TrimSpace(task.UpstreamResultURL)
	if mediaURL == "" {
		// Legacy fallback for rows created before upstream_result_url existed.
		mediaURL = OpenAIVideoResultURLFromBody([]byte(task.ResponseBody))
	}
	if strings.TrimSpace(mediaURL) == "" {
		return false, nil
	}
	if strings.HasPrefix(strings.TrimSpace(mediaURL), "/") {
		return false, nil
	}
	validatedURL, err := validateOpenAIVideoContentProxyURL(mediaURL)
	if err != nil {
		return true, err
	}
	if s == nil || s.httpUpstream == nil {
		return true, fmt.Errorf("video content proxy is unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, validatedURL, nil)
	if err != nil {
		return true, fmt.Errorf("video content proxy request failed")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(req.Context()))
	req = req.WithContext(WithHTTPUpstreamResolvedIPValidation(req.Context()))
	if c != nil && c.Request != nil {
		for _, header := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
			if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
				req.Header.Set(header, value)
			}
		}
	}
	resp, err := s.httpUpstream.Do(req, "", task.AccountID, 1)
	if err != nil {
		return true, fmt.Errorf("video content proxy request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return true, fmt.Errorf("video content is unavailable")
	}
	if err := s.streamOpenAIVideoContentResponse(resp, c); err != nil {
		return true, err
	}
	return true, nil
}

func validateOpenAIVideoContentProxyURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	_, err := urlvalidator.ValidateHTTPSURL(rawURL, urlvalidator.ValidationOptions{})
	if err != nil {
		return "", fmt.Errorf("invalid video content url")
	}
	// Preserve the exact signed URL. The generic validator normalizes trailing
	// slashes, which can invalidate provider signatures even though validation
	// itself succeeds.
	return rawURL, nil
}

func (s *OpenAIGatewayService) handleOpenAIVideoErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, upstreamModel string) (*OpenAIForwardResult, error) {
	statusCode := resp.StatusCode
	body, err := readUpstreamResponseBodyLimited(resp.Body, resolveUpstreamResponseReadLimit(s.cfg))
	if err != nil {
		body = nil
	}
	s.handleOpenAIAccountUpstreamError(ctx, account, statusCode, resp.Header, body, upstreamModel)
	setOpsUpstreamError(c, statusCode, "video upstream request failed", "")
	return nil, &OpenAIImagesUpstreamError{
		StatusCode: statusCode,
		ErrorType:  "upstream_error",
		Message:    "Video upstream request failed",
	}
}

func extractOpenAIVideoTaskID(body []byte) string {
	for _, path := range []string{
		"id",
		"request_id",
		"task_id",
		"data.id",
		"data.request_id",
		"data.task_id",
		"data.task.id",
		"data.task.request_id",
		"data.task.task_id",
		"data.video.id",
		"data.result.id",
		"data.result.request_id",
		"data.result.task_id",
		"result.id",
		"result.request_id",
		"result.task_id",
		"raw_data.id",
		"raw_data.request_id",
		"raw_data.task_id",
		"data.raw_data.id",
		"data.raw_data.request_id",
		"data.raw_data.task_id",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func extractOpenAIVideoStatus(body []byte) string {
	for _, path := range []string{
		"status",
		"state",
		"data.status",
		"data.state",
		"data.task.status",
		"data.task.state",
		"data.video.status",
		"data.video.state",
		"data.result.status",
		"data.result.state",
		"result.status",
		"result.state",
		"video.status",
		"video.state",
		"raw_data.status",
		"raw_data.state",
		"data.raw_data.status",
		"data.raw_data.state",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return NormalizeMediaGenerationStatus(value)
		}
	}
	return ""
}

func openAIVideoHasResultURL(body []byte) bool {
	return OpenAIVideoResultURLFromBody(body) != ""
}

func OpenAIVideoResultURLFromBody(body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range openAIVideoResultURLPaths() {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	for _, path := range openAIVideoResultURLArrayPaths() {
		for _, item := range gjson.GetBytes(body, path).Array() {
			if item.Type == gjson.String {
				if value := strings.TrimSpace(item.String()); value != "" {
					return value
				}
			}
			if value := strings.TrimSpace(item.Get("video_url").String()); value != "" {
				return value
			}
			if value := strings.TrimSpace(item.Get("url").String()); value != "" {
				return value
			}
			if value := strings.TrimSpace(item.Get("result_url").String()); value != "" {
				return value
			}
		}
	}
	return ""
}

func RewriteOpenAIVideoClientResponseBody(body []byte, taskID string, upstreamTaskIDs ...string) []byte {
	return RewriteOpenAIVideoClientResponseBodyWithBaseURL(body, taskID, "", upstreamTaskIDs...)
}

func RewriteOpenAIVideoClientResponseBodyWithBaseURL(body []byte, taskID, publicBaseURL string, upstreamTaskIDs ...string) []byte {
	return RewriteOpenAIVideoClientResponseBodyWithContentURL(
		body,
		taskID,
		openAIVideoContentProxyURL(publicBaseURL, taskID),
		upstreamTaskIDs...,
	)
}

func RewriteOpenAIVideoClientResponseBodyWithContentURL(body []byte, taskID, contentURL string, upstreamTaskIDs ...string) []byte {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return []byte(`{"status":"processing"}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		safe, _ := json.Marshal(map[string]any{"id": taskID, "status": MediaGenerationStatusRunning})
		return safe
	}
	state := openAIVideoClientRewriteState{
		publicTaskID:    taskID,
		contentURL:      strings.TrimSpace(contentURL),
		upstreamTaskIDs: make([]string, 0, len(upstreamTaskIDs)),
	}
	for _, upstreamTaskID := range upstreamTaskIDs {
		if value := strings.TrimSpace(upstreamTaskID); value != "" && value != taskID {
			state.upstreamTaskIDs = append(state.upstreamTaskIDs, value)
		}
	}
	payload = state.rewrite(payload, "", "")
	if object, ok := payload.(map[string]any); ok && !state.rewroteTaskID {
		object["id"] = taskID
	}
	rewritten, err := json.Marshal(payload)
	if err != nil {
		safe, _ := json.Marshal(map[string]any{"id": taskID, "status": MediaGenerationStatusRunning})
		return safe
	}
	return rewritten
}

type openAIVideoClientRewriteState struct {
	publicTaskID    string
	contentURL      string
	upstreamTaskIDs []string
	rewroteTaskID   bool
}

func (s *openAIVideoClientRewriteState) rewrite(value any, path, parentKey string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if isOpenAIVideoTaskIDPath(childPath) {
				typed[key] = s.publicTaskID
				s.rewroteTaskID = true
				continue
			}
			typed[key] = s.rewrite(child, childPath, key)
		}
		return typed
	case []any:
		for index, child := range typed {
			typed[index] = s.rewrite(child, path, parentKey)
		}
		return typed
	case string:
		return s.rewriteString(typed, parentKey, path)
	default:
		return value
	}
}

func (s *openAIVideoClientRewriteState) rewriteString(value, key, path string) string {
	value = strings.TrimSpace(value)
	for _, upstreamTaskID := range s.upstreamTaskIDs {
		value = strings.ReplaceAll(value, upstreamTaskID, s.publicTaskID)
	}
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	if isOpenAIVideoResultURLKey(lowerKey) {
		if value != "" {
			return s.contentURL
		}
		return value
	}
	if isOpenAIVideoResultURLArrayPath(path) && isOpenAIVideoExternalURL(value) {
		return s.contentURL
	}
	if isOpenAIVideoSensitiveLocationKey(lowerKey) {
		return ""
	}
	if isOpenAIVideoExternalURL(value) {
		return ""
	}
	if strings.HasPrefix(value, "//") || strings.Contains(value, "://") {
		return ""
	}
	return value
}

func isOpenAIVideoExternalURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) && parsed.Host != ""
}

func isOpenAIVideoResultURLArrayPath(path string) bool {
	for _, candidate := range openAIVideoResultURLArrayPaths() {
		if path == candidate {
			return true
		}
	}
	return false
}

func isOpenAIVideoTaskIDPath(path string) bool {
	for _, candidate := range openAIVideoTaskIDPaths() {
		if path == candidate {
			return true
		}
	}
	return false
}

func isOpenAIVideoResultURLKey(key string) bool {
	switch key {
	case "url", "video_url", "result_url", "content_url", "download_url", "file_url", "output_url":
		return true
	default:
		return false
	}
}

func isOpenAIVideoSensitiveLocationKey(key string) bool {
	if strings.Contains(key, "upstream") {
		return true
	}
	switch key {
	case "location", "host", "hostname", "path", "endpoint", "status_url", "task_url":
		return true
	default:
		return strings.HasSuffix(key, "_url")
	}
}

// SanitizeOpenAIVideoStoredResponseBody removes upstream failure payloads
// before persistence. Successful and in-progress bodies are subsequently run
// through RewriteOpenAIVideoClientResponseBody; signed media URLs are retained
// only in MediaGenerationTask.UpstreamResultURL.
func SanitizeOpenAIVideoStoredResponseBody(body []byte, status string) []byte {
	if !IsMediaGenerationFailureStatus(status) {
		return body
	}
	safeBody := []byte(`{"status":"failed","error":{"message":"Video generation failed","type":"upstream_error","param":null,"code":null}}`)
	normalized := NormalizeMediaGenerationStatus(status)
	if normalized != "" {
		safeBody, _ = sjson.SetBytes(safeBody, "status", normalized)
	}
	return safeBody
}

func openAIVideoTaskIDPaths() []string {
	return []string{
		"id",
		"request_id",
		"task_id",
		"data.id",
		"data.request_id",
		"data.task_id",
		"data.task.id",
		"data.task.request_id",
		"data.task.task_id",
		"data.video.id",
		"data.video.request_id",
		"data.video.task_id",
		"data.result.id",
		"data.result.request_id",
		"data.result.task_id",
		"result.id",
		"result.request_id",
		"result.task_id",
		"result.task.id",
		"result.task.request_id",
		"result.task.task_id",
		"video.id",
		"video.request_id",
		"video.task_id",
		"raw_data.id",
		"raw_data.request_id",
		"raw_data.task_id",
	}
}

func openAIVideoResultURLPaths() []string {
	return []string{
		"url",
		"video_url",
		"result_url",
		"content.url",
		"content.video_url",
		"content.result_url",
		"data.url",
		"data.video_url",
		"data.result_url",
		"data.content.url",
		"data.content.video_url",
		"data.content.result_url",
		"data.video.url",
		"data.video.video_url",
		"data.video.result_url",
		"data.result.url",
		"data.result.video_url",
		"data.result.result_url",
		"data.result.content.url",
		"data.result.content.video_url",
		"data.result.content.result_url",
		"result.url",
		"result.video_url",
		"result.result_url",
		"result.content.url",
		"result.content.video_url",
		"result.content.result_url",
		"video.url",
		"video.video_url",
		"video.result_url",
		"raw_data.url",
		"raw_data.video_url",
		"raw_data.result_url",
		"raw_data.content_url",
		"data.raw_data.url",
		"data.raw_data.video_url",
		"data.raw_data.result_url",
		"data.raw_data.content_url",
	}
}

func openAIVideoResultURLArrayPaths() []string {
	return []string{"data", "output", "data.output", "result.output", "data.result.output"}
}

func openAIVideoContentProxyPath(taskID string) string {
	return "/v1/videos/" + url.PathEscape(strings.TrimSpace(taskID)) + "/content"
}

func openAIVideoContentProxyURL(publicBaseURL, taskID string) string {
	path := openAIVideoContentProxyPath(taskID)
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if publicBaseURL == "" {
		return path
	}
	parsed, err := url.Parse(publicBaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return path
	}
	basePath := strings.TrimRight(parsed.EscapedPath(), "/")
	if strings.HasSuffix(strings.ToLower(basePath), "/v1") {
		path = strings.TrimPrefix(path, "/v1")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = ""
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/") + basePath + path
}

func (s *OpenAIGatewayService) OpenAIVideoClientContentURL(ctx context.Context, publicBaseURL, taskID, upstreamResultURL string) (string, error) {
	return s.openAIVideoClientContentURL(ctx, publicBaseURL, taskID, upstreamResultURL, nil)
}

func (s *OpenAIGatewayService) OpenAIVideoClientContentURLForTask(ctx context.Context, publicBaseURL string, task *MediaGenerationTask) (string, error) {
	if task == nil {
		return "", fmt.Errorf("video task is required")
	}
	upstreamResultURL := strings.TrimSpace(task.UpstreamResultURL)
	if upstreamResultURL == "" {
		upstreamResultURL = OpenAIVideoResultURLFromBody([]byte(task.ResponseBody))
	}
	return s.openAIVideoClientContentURL(ctx, publicBaseURL, task.ClientTaskID(), upstreamResultURL, task)
}

func (s *OpenAIGatewayService) openAIVideoClientContentURL(ctx context.Context, publicBaseURL, taskID, upstreamResultURL string, task *MediaGenerationTask) (string, error) {
	if s == nil || s.cfg == nil || s.cfg.Gateway.VideoProxy.Mode != config.VideoProxyModeEdge {
		return openAIVideoContentProxyURL(publicBaseURL, taskID), nil
	}
	validatedURL, edgeHeaders, err := s.resolveOpenAIVideoEdgeTarget(ctx, upstreamResultURL, task)
	if err != nil {
		return "", err
	}
	if validatedURL == "" {
		return "", nil
	}
	return s.openAIMediaEdgeProxyURL(ctx, "video", validatedURL, edgeHeaders, false)
}

func (s *OpenAIGatewayService) openAIMediaEdgeProxyURL(
	ctx context.Context,
	mediaType string,
	upstreamURL string,
	headers map[string]string,
	allowInsecureHTTP bool,
) (string, error) {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType != "video" && mediaType != "image" {
		return "", fmt.Errorf("media edge proxy is unavailable")
	}
	upstreamURL = strings.TrimSpace(upstreamURL)
	if len(upstreamURL) == 0 || len(upstreamURL) > 8192 || strings.ContainsAny(upstreamURL, "\r\n") {
		return "", fmt.Errorf("invalid %s content url", mediaType)
	}
	if _, err := urlvalidator.ValidateHTTPURL(upstreamURL, allowInsecureHTTP, urlvalidator.ValidationOptions{}); err != nil {
		return "", fmt.Errorf("invalid %s content url", mediaType)
	}
	parsedUpstreamURL, err := url.Parse(upstreamURL)
	if err != nil || parsedUpstreamURL.Hostname() == "" || parsedUpstreamURL.User != nil || parsedUpstreamURL.Fragment != "" {
		return "", fmt.Errorf("invalid %s content url", mediaType)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := urlvalidator.ResolvePublicIPs(resolveCtx, parsedUpstreamURL.Hostname()); err != nil {
		return "", fmt.Errorf("invalid %s content url", mediaType)
	}
	cfg := s.cfg.Gateway.VideoProxy
	key, err := hex.DecodeString(strings.TrimSpace(cfg.EncryptionKey))
	if err != nil || len(key) != 32 {
		return "", fmt.Errorf("media edge proxy is unavailable")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("media edge proxy is unavailable")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("media edge proxy is unavailable")
	}
	payload, err := json.Marshal(openAIVideoEdgeTokenPayload{
		Version:   1,
		URL:       upstreamURL,
		ExpiresAt: time.Now().UTC().Add(time.Duration(cfg.TokenTTLSeconds) * time.Second).Unix(),
		Headers:   headers,
		MediaType: mediaType,
	})
	if err != nil {
		return "", fmt.Errorf("media edge proxy is unavailable")
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("media edge proxy is unavailable")
	}
	sealed := gcm.Seal(nonce, nonce, payload, []byte(openAIVideoEdgeTokenAAD))
	token := base64.RawURLEncoding.EncodeToString(sealed)
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.EdgeBaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("media edge proxy is unavailable")
	}
	return baseURL + "/v1/" + mediaType + "-content/" + token, nil
}

func (s *OpenAIGatewayService) resolveOpenAIVideoEdgeTarget(ctx context.Context, upstreamResultURL string, task *MediaGenerationTask) (string, map[string]string, error) {
	upstreamResultURL = strings.TrimSpace(upstreamResultURL)
	if isOpenAIVideoExternalURL(upstreamResultURL) {
		validatedURL, err := validateOpenAIVideoContentProxyURL(upstreamResultURL)
		return validatedURL, nil, err
	}
	if task == nil {
		if upstreamResultURL == "" {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("invalid video content url")
	}
	providerTaskID := task.ProviderTaskID()
	endpoint := normalizeOpenAIVideoTaskEndpoint(task.UpstreamEndpoint)
	if providerTaskID == "" || endpoint == "" || task.AccountID <= 0 {
		return "", nil, fmt.Errorf("video edge proxy target is unavailable")
	}
	if s == nil || s.accountRepo == nil {
		return "", nil, fmt.Errorf("video edge proxy account is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	account, err := s.accountRepo.GetByID(ctx, task.AccountID)
	if err != nil || account == nil || account.Type != AccountTypeAPIKey {
		return "", nil, fmt.Errorf("video edge proxy account is unavailable")
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		return "", nil, fmt.Errorf("video edge proxy account base_url is unavailable")
	}
	validatedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", nil, fmt.Errorf("video edge proxy account base_url is unavailable")
	}
	contentPath := strings.TrimRight(endpoint, "/") + "/" + url.PathEscape(providerTaskID) + "/content"
	targetURL := buildOpenAIEndpointURL(validatedBaseURL, contentPath)
	validatedTargetURL, err := validateOpenAIVideoContentProxyURL(targetURL)
	if err != nil {
		return "", nil, err
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return "", nil, fmt.Errorf("video edge proxy authorization is unavailable")
	}
	authRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, validatedTargetURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("video edge proxy authorization is unavailable")
	}
	applyOpenAICompatibleAPIKeyAuth(authRequest, account, token)
	headers := make(map[string]string, 4)
	for _, name := range []string{
		OpenAICompatibleAuthHeaderAuthorization,
		OpenAICompatibleAuthHeaderAPIKey,
		OpenAICompatibleAuthHeaderXAPIKey,
		"X-Goog-API-Key",
	} {
		if value := strings.TrimSpace(authRequest.Header.Get(name)); value != "" {
			headers[name] = value
		}
	}
	if len(headers) == 0 {
		return "", nil, fmt.Errorf("video edge proxy authorization is unavailable")
	}
	return validatedTargetURL, headers, nil
}

func (s *OpenAIGatewayService) OpenAIVideoEdgeProxyEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.VideoProxy.Mode == config.VideoProxyModeEdge
}

func NewOpenAIVideoPublicTaskID() string {
	return "video-" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func localOpenAIVideoSynchronousTaskID(upstreamRequestID string, body []byte) string {
	seed := strings.TrimSpace(upstreamRequestID)
	if seed == "" {
		seed = fmt.Sprintf("local:%d:%s", time.Now().UnixNano(), string(body))
	} else {
		seed = "upstream:" + seed
	}
	sum := sha256.Sum256([]byte(seed))
	return "video-sync-" + hex.EncodeToString(sum[:])[:24]
}

func extractOpenAIVideoDurationSeconds(body []byte) int {
	for _, path := range []string{
		"duration",
		"seconds",
		"duration_seconds",
		"video_duration",
		"sora2_duration",
		"data.duration",
		"data.seconds",
		"data.duration_seconds",
		"data.task.duration",
		"data.task.seconds",
		"data.task.duration_seconds",
		"data.video.duration",
		"data.video.seconds",
		"data.video.duration_seconds",
		"data.result.duration",
		"data.result.seconds",
		"data.result.duration_seconds",
		"video.duration",
		"video.seconds",
		"video.duration_seconds",
		"result.duration",
		"result.seconds",
		"result.duration_seconds",
		"raw_data.duration",
		"raw_data.seconds",
		"raw_data.duration_seconds",
		"data.raw_data.duration",
		"data.raw_data.seconds",
		"data.raw_data.duration_seconds",
	} {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			continue
		}
		if seconds := normalizeOpenAIVideoDurationSeconds(value.String()); seconds > 0 {
			return seconds
		}
	}
	return 0
}

func normalizeOpenAIVideoDurationSeconds(value string) int {
	value = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "s"))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	seconds := int(parsed)
	if seconds < 0 {
		return 0
	}
	if seconds > 3600 {
		return 3600
	}
	return seconds
}

func (s *OpenAIGatewayService) openAIMediaTaskRepo() (MediaGenerationTaskRepository, bool) {
	if s == nil || s.usageBillingRepo == nil {
		return nil, false
	}
	repo, ok := s.usageBillingRepo.(MediaGenerationTaskRepository)
	return repo, ok
}

func (s *OpenAIGatewayService) OpenAIVideoPublicBaseURL(ctx context.Context) string {
	if s == nil || s.settingService == nil {
		return ""
	}
	return s.settingService.GetAPIBaseURL(ctx)
}

func (s *OpenAIGatewayService) GetOpenAIVideoTaskByTaskID(ctx context.Context, apiKeyID int64, taskID string) (*MediaGenerationTask, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.GetMediaGenerationTaskByTaskID(ctx, apiKeyID, taskID)
}

func (s *OpenAIGatewayService) GetOpenAIVideoTaskByIdempotency(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (*MediaGenerationTask, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.GetMediaGenerationTaskByIdempotency(ctx, apiKeyID, idempotencyKeyHash)
}

func (s *OpenAIGatewayService) AcquireOpenAIVideoIdempotencyLock(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (func(), error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.AcquireMediaGenerationIdempotencyLock(ctx, apiKeyID, idempotencyKeyHash)
}

func (s *OpenAIGatewayService) CreateOpenAIVideoTask(ctx context.Context, task *MediaGenerationTask) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.CreateMediaGenerationTask(ctx, task)
}

func (s *OpenAIGatewayService) UpdateOpenAIVideoTaskResponse(ctx context.Context, apiKeyID int64, taskID string, result *OpenAIForwardResult) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok || result == nil {
		if result == nil {
			return nil
		}
		return fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.UpdateMediaGenerationTaskResponse(
		ctx,
		apiKeyID,
		taskID,
		result.ResponseStatus,
		result.ResponseContentType,
		string(result.ResponseBody),
		result.MediaResultURL,
		result.VideoStatus,
		result.MediaDurationSeconds,
	)
}

func (s *OpenAIGatewayService) MarkOpenAIVideoTaskTerminal(ctx context.Context, apiKeyID int64, taskID, status, finalizationError string) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.MarkMediaGenerationTaskTerminal(ctx, apiKeyID, taskID, status, finalizationError)
}

func (s *OpenAIGatewayService) TryAcquireOpenAIVideoFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken string, leaseUntil time.Time) (bool, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return false, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.TryAcquireMediaGenerationFinalization(ctx, apiKeyID, taskID, leaseToken, leaseUntil)
}

func (s *OpenAIGatewayService) CompleteOpenAIVideoFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken string) (bool, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return false, fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.CompleteMediaGenerationFinalization(ctx, apiKeyID, taskID, leaseToken)
}

func (s *OpenAIGatewayService) ReleaseOpenAIVideoFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken, finalizationError string) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return fmt.Errorf("media generation task repository is unavailable")
	}
	return repo.ReleaseMediaGenerationFinalization(ctx, apiKeyID, taskID, leaseToken, finalizationError)
}

func OpenAIVideoTaskSessionHash(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ""
	}
	return "openai-video:" + DeriveSessionHashFromSeed(requestID)
}

func (s *OpenAIGatewayService) BindOpenAIVideoTaskAccount(ctx context.Context, groupID *int64, requestID string, accountID int64) error {
	return s.BindStickySession(ctx, groupID, OpenAIVideoTaskSessionHash(requestID), accountID)
}

func boolToMediaCount(ok bool) int {
	if ok {
		return 1
	}
	return 0
}
