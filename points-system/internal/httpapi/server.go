package httpapi

import (
	"context"
	"embed"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/config"
	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/security"
	"github.com/hxly520/sub2api/points-system/internal/store"
)

//go:embed web/* web/assets/*
var webFS embed.FS

type Server struct {
	Config        config.Config
	Store         *store.Store
	Logger        *slog.Logger
	limits        *security.RateLimiter
	mux           *http.ServeMux
	trustedProxy  *net.IPNet
	sessionLookup func(context.Context, string, time.Time) (store.Session, error)
	policyLookup  func(context.Context, time.Time) (domain.Policy, error)
}

type principal struct {
	Session store.Session
	Token   string
}

type contextKey string

const principalKey contextKey = "points-principal"
const policyKey contextKey = "points-policy"

const embeddedUIMode = "embedded"

func New(cfg config.Config, pointsStore *store.Store, logger *slog.Logger) (*Server, error) {
	if pointsStore == nil {
		return nil, errors.New("HTTP API store is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	var trustedProxy *net.IPNet
	if raw := strings.TrimSpace(cfg.TrustedProxyCIDR); raw != "" {
		_, parsed, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, errors.New("POINTS_TRUSTED_PROXY_CIDR must be a valid CIDR")
		}
		trustedProxy = parsed
	}
	s := &Server{Config: cfg, Store: pointsStore, Logger: logger, limits: security.NewRateLimiter(), mux: http.NewServeMux(), trustedProxy: trustedProxy}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.recoverer(s.securityHeaders(s.requestID(s.mux)))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /launch", s.launch)
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	s.mux.Handle("GET /app/", s.auth("user", false, true, http.HandlerFunc(s.userPage)))
	s.mux.Handle("GET /admin/", s.auth("admin", false, false, http.HandlerFunc(s.adminPage)))
	s.mux.Handle("GET /assets/app.css", s.auth("", false, true, http.HandlerFunc(s.webAsset("web/assets/app.css", "text/css; charset=utf-8"))))
	s.mux.Handle("GET /assets/common.js", s.auth("", false, true, http.HandlerFunc(s.webAsset("web/assets/common.js", "text/javascript; charset=utf-8"))))
	s.mux.Handle("GET /assets/user.js", s.auth("user", false, true, http.HandlerFunc(s.webAsset("web/assets/user.js", "text/javascript; charset=utf-8"))))
	s.mux.Handle("GET /assets/admin.js", s.auth("admin", false, false, http.HandlerFunc(s.webAsset("web/assets/admin.js", "text/javascript; charset=utf-8"))))

	s.mux.Handle("GET /api/v1/me", s.auth("user", false, true, http.HandlerFunc(s.me)))
	s.mux.Handle("GET /api/v1/ledger", s.auth("user", false, true, http.HandlerFunc(s.ledger)))
	s.mux.Handle("GET /api/v1/daily-points", s.auth("user", false, true, http.HandlerFunc(s.dailyPoints)))
	s.mux.Handle("POST /api/v1/checkins", s.auth("user", true, true, s.rate("checkin", 6, time.Minute, http.HandlerFunc(s.checkin))))
	s.mux.Handle("GET /api/v1/balance-grants", s.auth("user", false, true, http.HandlerFunc(s.checkinBalanceGrants)))
	s.mux.Handle("POST /api/v1/logout", s.auth("", true, false, http.HandlerFunc(s.logout)))

	s.mux.Handle("GET /api/v1/admin/me", s.auth("admin", false, false, http.HandlerFunc(s.adminMe)))
	s.mux.Handle("GET /api/v1/admin/users/points", s.auth("admin", false, false, http.HandlerFunc(s.adminUserPoints)))
	s.mux.Handle("GET /api/v1/admin/policies", s.auth("admin", false, false, http.HandlerFunc(s.policies)))
	s.mux.Handle("POST /api/v1/admin/policies", s.auth("admin", true, false, http.HandlerFunc(s.createPolicy)))
	s.mux.Handle("GET /api/v1/admin/balance-grants", s.auth("admin", false, false, http.HandlerFunc(s.adminCheckinBalanceGrants)))
	s.mux.Handle("POST /api/v1/admin/balance-grants/{id}/retry", s.auth("admin", true, false, http.HandlerFunc(s.retryCheckinBalanceGrant)))
	s.mux.Handle("POST /api/v1/admin/balance-grants/{id}/reverse", s.auth("admin", true, false, http.HandlerFunc(s.reverseCheckinBalanceGrant)))

}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "Service unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) userPage(w http.ResponseWriter, _ *http.Request) {
	s.servePage(w, "web/user.html")
}

func (s *Server) adminPage(w http.ResponseWriter, _ *http.Request) {
	s.servePage(w, "web/admin.html")
}

func (s *Server) servePage(w http.ResponseWriter, name string) {
	body, err := webFS.ReadFile(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "asset_error", "Internal server error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *Server) webAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		body, err := webFS.ReadFile(name)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Not found")
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	if !s.limits.Allow("launch:"+s.clientIP(r), 20, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests")
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if raw == "" || len(raw) > 4096 {
		writeError(w, http.StatusBadRequest, "invalid_ticket", "Invalid launch ticket")
		return
	}
	now := time.Now().UTC()
	claims, err := security.ValidateLaunchTicket(raw, s.Config.LaunchIssuer, s.Config.LaunchAudience, s.Config.LaunchKeys, now)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_ticket", "Invalid launch ticket")
		return
	}
	if claims.Role != "admin" {
		policy, policyErr := s.currentPolicy(r.Context(), now)
		allowed := false
		if policyErr == nil {
			allowed, policyErr = s.userPolicyAllowsAccess(r.Context(), policy)
		}
		if errors.Is(policyErr, domain.ErrNotFound) || (policyErr == nil && !allowed) {
			writeError(w, http.StatusForbidden, "points_disabled", "Points system is disabled")
			return
		}
		if policyErr != nil {
			s.fail(w, r, policyErr)
			return
		}
	}
	token, err := security.RandomToken(32)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := s.Store.ConsumeLaunchTicket(r.Context(), claims, token, s.Config.SessionTTL, now); err != nil {
		writeError(w, http.StatusUnauthorized, "ticket_consumed", "Invalid launch ticket")
		return
	}
	s.setSessionCookie(w, token, now.Add(s.Config.SessionTTL))
	destination := workspaceDestination(claims.Role, requestedUIMode(r))
	http.Redirect(w, r, destination, http.StatusSeeOther)
}

func requestedUIMode(r *http.Request) string {
	if r != nil && r.URL.Query().Get("ui_mode") == embeddedUIMode {
		return embeddedUIMode
	}
	return ""
}

func workspaceDestination(role, uiMode string) string {
	destination := "/app/"
	if role == "admin" {
		destination = "/admin/"
	}
	if uiMode == embeddedUIMode {
		return destination + "?ui_mode=" + embeddedUIMode
	}
	return destination
}

func (s *Server) auth(requiredRole string, csrf, requireEnabled bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.cookieName())
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		now := time.Now().UTC()
		session, err := s.lookupSession(r.Context(), cookie.Value, now)
		if err != nil {
			s.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		if requiredRole != "" && session.Role != requiredRole {
			writeError(w, http.StatusForbidden, "forbidden", "Forbidden")
			return
		}
		if requiredRole == "" && session.Role != "user" && session.Role != "admin" {
			writeError(w, http.StatusForbidden, "forbidden", "Forbidden")
			return
		}
		ctx := context.WithValue(r.Context(), principalKey, principal{Session: session, Token: cookie.Value})
		if requireEnabled && session.Role == "user" {
			policy, policyErr := s.currentPolicy(ctx, now)
			allowed := false
			if policyErr == nil {
				allowed, policyErr = s.userPolicyAllowsAccess(ctx, policy)
			}
			if errors.Is(policyErr, domain.ErrNotFound) || (policyErr == nil && !allowed) {
				writeError(w, http.StatusForbidden, "points_disabled", "Points system is disabled")
				return
			}
			if policyErr != nil {
				s.fail(w, r, policyErr)
				return
			}
			ctx = context.WithValue(ctx, policyKey, policy)
		}
		if csrf {
			if r.Header.Get("Origin") != s.Config.PublicOrigin {
				writeError(w, http.StatusForbidden, "origin_mismatch", "Forbidden")
				return
			}
			if !security.VerifyCSRF(cookie.Value, r.Header.Get("X-CSRF-Token"), s.Config.SessionSecret) {
				writeError(w, http.StatusForbidden, "csrf_invalid", "Forbidden")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) lookupSession(ctx context.Context, token string, now time.Time) (store.Session, error) {
	if s.sessionLookup != nil {
		return s.sessionLookup(ctx, token, now)
	}
	if s.Store == nil {
		return store.Session{}, domain.ErrNotFound
	}
	return s.Store.Session(ctx, token, now)
}

func (s *Server) currentPolicy(ctx context.Context, now time.Time) (domain.Policy, error) {
	date := now
	if s.Store != nil {
		date = s.Store.BusinessDate(now)
	} else if s.Config.Timezone != nil {
		local := now.In(s.Config.Timezone)
		date = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, s.Config.Timezone)
	}
	if s.policyLookup != nil {
		return s.policyLookup(ctx, date)
	}
	if s.Store == nil {
		return domain.Policy{}, domain.ErrNotFound
	}
	return s.Store.PolicyForDate(ctx, date)
}

func policyAllowsUserAccess(policy domain.Policy) bool {
	return policy.Enabled && policy.ValidateForEnable() == nil
}

func (s *Server) userPolicyAllowsAccess(ctx context.Context, policy domain.Policy) (bool, error) {
	if !policyAllowsUserAccess(policy) {
		return false, nil
	}
	if s.Store == nil {
		return true, nil
	}
	return s.Store.UserAccessReadyForPolicy(ctx, policy.VersionNo)
}

func policyFrom(r *http.Request) (domain.Policy, bool) {
	policy, ok := r.Context().Value(policyKey).(domain.Policy)
	return policy, ok
}

func (s *Server) rate(scope string, limit int, window time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := scope + ":" + s.clientIP(r)
		if p, ok := principalFrom(r); ok {
			key += ":" + strconv.FormatInt(p.Session.UserID, 10)
		}
		if !s.limits.Allow(key, limit, window) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func principalFrom(r *http.Request) (principal, bool) {
	p, ok := r.Context().Value(principalKey).(principal)
	return p, ok
}

func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remoteIP := net.ParseIP(strings.TrimSpace(host))
	if remoteIP == nil || s.trustedProxy == nil || !s.trustedProxy.Contains(remoteIP) {
		return host
	}
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(forwarded) > 0 {
		candidate := strings.TrimSpace(forwarded[0])
		if net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	return host
}

func (s *Server) cookieName() string {
	if s.Config.CookieSecure {
		return "__Host-points_session"
	}
	return "points_session"
}

func (s *Server) setSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: value, Path: "/", Expires: expires,
		MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true, Secure: s.Config.CookieSecure,
		SameSite: http.SameSiteStrictMode})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Path: "/", MaxAge: -1, Expires: time.Unix(1, 0),
		HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteStrictMode})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		frameAncestor := s.Config.EmbedParentOrigin
		if frameAncestor == "" {
			frameAncestor = "'none'"
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors "+frameAncestor+"; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if len(requestID) < 8 || len(requestID) > 128 {
			requestID, _ = security.RandomToken(16)
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.Logger.Error("HTTP handler panic", "panic", recovered, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.Logger.Error("points API request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeDomainError(w, err)
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Forbidden")
	case errors.Is(err, domain.ErrPolicyIncomplete):
		writeError(w, http.StatusUnprocessableEntity, "policy_incomplete", "Policy is incomplete")
	case errors.Is(err, domain.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, "idempotency_conflict", "Idempotency key conflict")
	case errors.Is(err, domain.ErrSnapshotNotReady):
		writeError(w, http.StatusServiceUnavailable, "snapshot_not_ready", "Yesterday points are still being prepared")
	case errors.Is(err, domain.ErrDisabled), errors.Is(err, domain.ErrCapExhausted),
		errors.Is(err, domain.ErrCheckinLimit),
		errors.Is(err, domain.ErrCheckinSpendMinimum), errors.Is(err, domain.ErrNoMatchingTier),
		errors.Is(err, domain.ErrInvalidState):
		writeError(w, http.StatusConflict, "business_rule", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
	}
}
