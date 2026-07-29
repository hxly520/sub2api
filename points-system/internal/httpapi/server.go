package httpapi

import (
	"context"
	"embed"
	"errors"
	"io/fs"
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
	Config       config.Config
	Store        *store.Store
	Logger       *slog.Logger
	limits       *security.RateLimiter
	mux          *http.ServeMux
	trustedProxy *net.IPNet
}

type principal struct {
	Session store.Session
	Token   string
}

type contextKey string

const principalKey contextKey = "points-principal"

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
	assets, _ := fs.Sub(webFS, "web")
	s.mux.Handle("GET /assets/", http.FileServer(http.FS(assets)))
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /launch", s.launch)
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	s.mux.Handle("GET /app/", s.auth(false, false, http.HandlerFunc(s.page)))
	s.mux.Handle("GET /admin/", s.auth(true, false, http.HandlerFunc(s.page)))

	s.mux.Handle("GET /api/v1/me", s.auth(false, false, http.HandlerFunc(s.me)))
	s.mux.Handle("GET /api/v1/ledger", s.auth(false, false, http.HandlerFunc(s.ledger)))
	s.mux.Handle("POST /api/v1/checkins", s.auth(false, true, s.rate("checkin", 6, time.Minute, http.HandlerFunc(s.checkin))))
	s.mux.Handle("GET /api/v1/balance-grants", s.auth(false, false, http.HandlerFunc(s.balanceGrants)))
	s.mux.Handle("POST /api/v1/logout", s.auth(false, true, http.HandlerFunc(s.logout)))

	s.mux.Handle("GET /api/v1/admin/policies", s.auth(true, false, http.HandlerFunc(s.policies)))
	s.mux.Handle("POST /api/v1/admin/policies", s.auth(true, true, http.HandlerFunc(s.createPolicy)))
	s.mux.Handle("POST /api/v1/admin/grants", s.auth(true, true, http.HandlerFunc(s.manualGrant)))
	s.mux.Handle("GET /api/v1/admin/balance-grants", s.auth(true, false, http.HandlerFunc(s.adminBalanceGrants)))
	s.mux.Handle("POST /api/v1/admin/balance-grants/{id}/retry", s.auth(true, true, http.HandlerFunc(s.retryBalanceGrant)))
	s.mux.Handle("POST /api/v1/admin/balance-grants/{id}/reverse", s.auth(true, true, http.HandlerFunc(s.reverseBalanceGrant)))
	s.mux.Handle("POST /api/v1/admin/snapshots/refresh", s.auth(true, true, http.HandlerFunc(s.refreshSnapshots)))

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

func (s *Server) page(w http.ResponseWriter, _ *http.Request) {
	body, err := webFS.ReadFile("web/index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "asset_error", "Internal server error")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
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
	destination := "/app/"
	if claims.Role == "admin" {
		destination = "/admin/"
	}
	http.Redirect(w, r, destination, http.StatusSeeOther)
}

func (s *Server) auth(admin, csrf bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.cookieName())
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		session, err := s.Store.Session(r.Context(), cookie.Value, time.Now().UTC())
		if err != nil {
			s.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		if admin && session.Role != "admin" {
			writeError(w, http.StatusForbidden, "forbidden", "Forbidden")
			return
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
		ctx := context.WithValue(r.Context(), principalKey, principal{Session: session, Token: cookie.Value})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
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
