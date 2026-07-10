package service

import "context"

type httpUpstreamRedirectPolicyContextKey struct{}
type httpUpstreamResolvedIPValidationContextKey struct{}

// WithHTTPUpstreamRedirectsDisabled marks a single outbound request as
// non-redirectable. The repository clones the cached http.Client for that
// request, so the shared client and all other upstream traffic are unaffected.
func WithHTTPUpstreamRedirectsDisabled(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, httpUpstreamRedirectPolicyContextKey{}, true)
}

// HTTPUpstreamRedirectsDisabled reports whether redirects must be rejected for
// this outbound request.
func HTTPUpstreamRedirectsDisabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	disabled, _ := ctx.Value(httpUpstreamRedirectPolicyContextKey{}).(bool)
	return disabled
}

// WithHTTPUpstreamResolvedIPValidation forces DNS resolution and private-range
// rejection for one outbound request even when the global URL allowlist is
// disabled. It is used by public media proxies, where an upstream-controlled
// signed URL must never be able to target local infrastructure.
func WithHTTPUpstreamResolvedIPValidation(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, httpUpstreamResolvedIPValidationContextKey{}, true)
}

// HTTPUpstreamResolvedIPValidationRequired reports whether a request requires
// DNS/private-address validation regardless of the global URL policy.
func HTTPUpstreamResolvedIPValidationRequired(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	required, _ := ctx.Value(httpUpstreamResolvedIPValidationContextKey{}).(bool)
	return required
}
