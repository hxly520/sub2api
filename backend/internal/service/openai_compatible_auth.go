package service

import "net/http"

func applyOpenAICompatibleAPIKeyAuth(req *http.Request, account *Account, apiKey string) {
	req.Header.Del(OpenAICompatibleAuthHeaderAuthorization)
	req.Header.Del(OpenAICompatibleAuthHeaderAPIKey)
	req.Header.Del(OpenAICompatibleAuthHeaderXAPIKey)
	req.Header.Del("X-Goog-API-Key")

	switch account.OpenAICompatibleAuthHeader() {
	case OpenAICompatibleAuthHeaderAPIKey:
		req.Header.Set(OpenAICompatibleAuthHeaderAPIKey, apiKey)
	case OpenAICompatibleAuthHeaderXAPIKey:
		req.Header.Set(OpenAICompatibleAuthHeaderXAPIKey, apiKey)
	default:
		req.Header.Set(OpenAICompatibleAuthHeaderAuthorization, "Bearer "+apiKey)
	}
}
