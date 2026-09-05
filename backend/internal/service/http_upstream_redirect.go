package service

import "context"

type httpUpstreamResolvedIPValidationContextKey struct{}

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
