package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

const CCSwitchUsageAutoIntervalMinutes = 30

const ccSwitchUsageScript = `({
  request: {
    url: "{{baseUrl}}/v1/usage",
    method: "GET",
    headers: { "Authorization": "Bearer {{apiKey}}" }
  },
  extractor: function(response) {
    const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
    const unit = response?.unit ?? response?.quota?.unit ?? "USD";
    return {
      isValid: response?.is_active ?? response?.isValid ?? true,
      remaining,
      unit
    };
  }
})`

type CCSwitchUsageTemplate struct {
	Version             int    `json:"version"`
	ScriptBase64        string `json:"script_base64"`
	ScriptSHA256        string `json:"script_sha256"`
	EndpointPath        string `json:"endpoint_path"`
	AutoIntervalMinutes int    `json:"auto_interval_minutes"`
}

func GetCCSwitchUsageTemplate() CCSwitchUsageTemplate {
	digest := sha256.Sum256([]byte(ccSwitchUsageScript))
	return CCSwitchUsageTemplate{
		Version:             1,
		ScriptBase64:        base64.StdEncoding.EncodeToString([]byte(ccSwitchUsageScript)),
		ScriptSHA256:        hex.EncodeToString(digest[:]),
		EndpointPath:        "/v1/usage",
		AutoIntervalMinutes: CCSwitchUsageAutoIntervalMinutes,
	}
}
