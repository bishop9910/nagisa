package server

import (
	"context"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"
	"nagisa/internal/conf"
	"nagisa/internal/service"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/transport"
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/google/uuid"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(
	c *conf.Server,
	auth *service.AuthService,
	authUC *biz.AuthUsecase,
	user *service.UserService,
	node *service.NodeService,
	file *service.FileService,
	share *service.ShareService,
	audit *service.AuditService,
	system *service.SystemService,
) *kgrpc.Server {
	var opts = []kgrpc.ServerOption{
		kgrpc.Middleware(
			recovery.Recovery(),
			grpcAuth(authUC),
		),
	}
	if c.GetGrpc().GetNetwork() != "" {
		opts = append(opts, kgrpc.Network(c.GetGrpc().GetNetwork()))
	}
	if c.GetGrpc().GetAddr() != "" {
		opts = append(opts, kgrpc.Address(c.GetGrpc().GetAddr()))
	}
	if c.GetGrpc().GetTimeout() != nil {
		opts = append(opts, kgrpc.Timeout(c.GetGrpc().GetTimeout().AsDuration()))
	}
	srv := kgrpc.NewServer(opts...)

	v1.RegisterAuthServiceServer(srv, auth)
	v1.RegisterUserServiceServer(srv, user)
	v1.RegisterNodeServiceServer(srv, node)
	v1.RegisterFileServiceServer(srv, file)
	v1.RegisterShareServiceServer(srv, share)
	v1.RegisterAuditServiceServer(srv, audit)
	v1.RegisterSystemServiceServer(srv, system)
	return srv
}

// grpcPublicMethods lists the gRPC methods that need no access token.
var grpcPublicMethods = map[string]struct{}{
	"/netdisk.v1.AuthService/GetAuthConfig":        {},
	"/netdisk.v1.AuthService/Login":                {},
	"/netdisk.v1.AuthService/RefreshToken":         {},
	"/netdisk.v1.ShareService/AccessShare":         {},
	"/netdisk.v1.ShareService/ListShareChildren":   {},
	"/netdisk.v1.ShareService/GetShareDownloadUrl": {},
	"/netdisk.v1.SystemService/GetSystemInfo":      {},
	"/netdisk.v1.SystemService/HealthCheck":        {},
}

// grpcAuth resolves the caller from the gRPC metadata and stores it in the
// context, mirroring the HTTP middleware so a usecase behaves identically on
// both transports.
func grpcAuth(auth *biz.AuthUsecase) middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			caller := biz.AnonymousCaller()
			operation := ""
			if carrier, ok := transport.FromServerContext(ctx); ok {
				operation = carrier.Operation()
				header := carrier.RequestHeader()
				caller.UserAgent = header.Get("user-agent")
				caller.RequestID = header.Get("x-request-id")
				if token := header.Get("authorization"); token != "" {
					if resolved, err := auth.Authenticate(ctx, bearer(token)); err == nil {
						resolved.UserAgent = caller.UserAgent
						resolved.RequestID = caller.RequestID
						caller = resolved
					}
				}
				if unlock := header.Get("x-node-token"); unlock != "" {
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
			ctx = biz.NewContext(ctx, caller)
			if _, public := grpcPublicMethods[operation]; public {
				return next(ctx, req)
			}
			if !caller.IsAuthenticated() {
				return nil, biz.ErrUnauthenticated
			}
			return next(ctx, req)
		}
	}
}
