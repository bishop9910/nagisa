// Package middleware holds the transport level concerns of the netdisk HTTP
// and gRPC servers: identity extraction, CORS and the audit trail.
package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"

	"nagisa/internal/biz"

	kratosmw "github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/google/uuid"
)

// PublicOperations lists the operations that may be called without an access
// token. The key is the fully qualified proto method, which is what both the
// HTTP and the gRPC transports report as the operation, so one list covers
// both. Every other route requires a valid bearer token.
var PublicOperations = map[string]struct{}{
	"/netdisk.v1.AuthService/GetAuthConfig":        {},
	"/netdisk.v1.AuthService/Login":                {},
	"/netdisk.v1.AuthService/GuestLogin":           {},
	"/netdisk.v1.AuthService/RefreshToken":         {},
	"/netdisk.v1.ShareService/AccessShare":         {},
	"/netdisk.v1.ShareService/ListShareChildren":   {},
	"/netdisk.v1.ShareService/GetShareDownloadUrl": {},
	"/netdisk.v1.SystemService/GetSystemInfo":      {},
	"/netdisk.v1.SystemService/HealthCheck":        {},
}

// Auth resolves the caller of a request and stores it in the context. A
// request for a protected operation without a usable token is rejected before
// it reaches a handler, so no service method has to guard itself.
func Auth(auth *biz.AuthUsecase) kratosmw.Middleware {
	return func(next kratosmw.Handler) kratosmw.Handler {
		return func(ctx context.Context, req any) (any, error) {
			caller := biz.AnonymousCaller()
			if carrier, ok := transport.FromServerContext(ctx); ok {
				if tr, ok := carrier.(*khttp.Transport); ok {
					r := tr.Request()
					caller.IP = ClientIP(r)
					caller.UserAgent = r.UserAgent()
					caller.RequestID = r.Header.Get("X-Request-Id")
					if token := BearerToken(r); token != "" {
						if resolved, err := auth.Authenticate(ctx, token); err == nil {
							resolved.IP = caller.IP
							resolved.UserAgent = caller.UserAgent
							resolved.RequestID = caller.RequestID
							caller = resolved
						}
					}
					if unlock := r.Header.Get("X-Node-Token"); unlock != "" {
						if ids, err := auth.ParseNodeToken(unlock); err == nil {
							if caller.Unlocked == nil {
								caller.Unlocked = map[uuid.UUID]bool{}
							}
							for _, id := range ids {
								caller.Unlocked[id] = true
							}
						}
					}
				}
			}
			ctx = biz.NewContext(ctx, caller)
			if _, public := PublicOperations[operationOf(ctx)]; public {
				return next(ctx, req)
			}
			if !caller.IsAuthenticated() {
				return nil, biz.ErrUnauthenticated
			}
			return next(ctx, req)
		}
	}
}

// operationOf returns the fully qualified method name of the current request.
func operationOf(ctx context.Context) string {
	if carrier, ok := transport.FromServerContext(ctx); ok {
		return carrier.Operation()
	}
	return ""
}

// BearerToken extracts the access token from an Authorization header.
func BearerToken(r *http.Request) string {
	raw := r.Header.Get("Authorization")
	if raw == "" {
		return ""
	}
	if len(raw) > 7 && strings.EqualFold(raw[:7], "bearer ") {
		return strings.TrimSpace(raw[7:])
	}
	return strings.TrimSpace(raw)
}

// ClientIP returns the best guess of the client address, honouring the
// forwarding headers a reverse proxy sets.
func ClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if real := strings.TrimSpace(r.Header.Get("X-Real-Ip")); real != "" {
		return real
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
