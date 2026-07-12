package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

// A gateway retry is useful only when the upstream explicitly rejected the
// request before generation. Two bounded cross-account attempts let a
// three-account group skip two definitively unavailable accounts without ever
// replaying an ambiguous, potentially billable request.
const openAIMaxAutomaticReplayAttempts = 2

type openAIRequestRetryBudget struct {
	used int
}

func (b *openAIRequestRetryBudget) tryConsume(account *service.Account, failoverErr *service.UpstreamFailoverError) bool {
	if b == nil || account == nil ||
		!failoverErr.CanSafelyReplayRequest() || b.used >= openAIMaxAutomaticReplayAttempts {
		return false
	}
	b.used++
	return true
}
