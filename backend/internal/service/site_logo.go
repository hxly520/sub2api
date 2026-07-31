package service

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

const SiteLogoMaxDecodedBytes = 2 << 20

var siteLogoMediaTypes = map[string]struct{}{
	"image/gif":  {},
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

type ParsedSiteLogo struct {
	DataURI   string
	MediaType string
	Body      []byte
}

// ParseSiteLogoDataURI is the single validation boundary for stored and
// public site logos. An empty value clears the logo and is therefore valid.
func ParseSiteLogoDataURI(raw string) (*ParsedSiteLogo, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	header, payload, found := strings.Cut(raw, ",")
	if !found {
		return nil, fmt.Errorf("site_logo must be a base64 image data URI")
	}
	parts := strings.Split(header, ";")
	if len(parts) != 2 || !strings.EqualFold(parts[1], "base64") {
		return nil, fmt.Errorf("site_logo must be a base64 image data URI")
	}
	rawMediaType := strings.ToLower(parts[0])
	if !strings.HasPrefix(rawMediaType, "data:") {
		return nil, fmt.Errorf("site_logo must be a base64 image data URI")
	}
	mediaType := strings.TrimPrefix(rawMediaType, "data:")
	if _, allowed := siteLogoMediaTypes[mediaType]; !allowed {
		return nil, fmt.Errorf("site_logo must be PNG, JPEG, WebP, or GIF")
	}

	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, fmt.Errorf("site_logo image data is empty")
	}
	if len(payload) > base64.StdEncoding.EncodedLen(SiteLogoMaxDecodedBytes) {
		return nil, fmt.Errorf("site_logo exceeds the %d byte limit", SiteLogoMaxDecodedBytes)
	}
	body, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(body) == 0 {
		return nil, fmt.Errorf("site_logo contains invalid base64 image data")
	}
	if len(body) > SiteLogoMaxDecodedBytes {
		return nil, fmt.Errorf("site_logo exceeds the %d byte limit", SiteLogoMaxDecodedBytes)
	}
	detectedType := strings.ToLower(strings.TrimSpace(strings.SplitN(http.DetectContentType(body), ";", 2)[0]))
	if detectedType != mediaType {
		return nil, fmt.Errorf("site_logo content does not match its declared media type")
	}

	return &ParsedSiteLogo{
		DataURI:   "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(body),
		MediaType: mediaType,
		Body:      body,
	}, nil
}
