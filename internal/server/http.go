package server

import (
	"net/http"
	"strings"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"
	"nagisa/internal/conf"
	"nagisa/internal/server/middleware"
	"nagisa/internal/service"

	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/middleware/validate"
	khttp "github.com/go-kratos/kratos/v3/transport/http"

	"go.einride.tech/aip/fieldbehavior"
	"google.golang.org/protobuf/proto"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(
	c *conf.Server,
	webCfg *conf.Web,
	authUC *biz.AuthUsecase,
	auth *service.AuthService,
	user *service.UserService,
	node *service.NodeService,
	file *service.FileService,
	share *service.ShareService,
	audit *service.AuditService,
	system *service.SystemService,
	stream *StreamHandler,
	auditor *middleware.Auditor,
) *khttp.Server {
	var opts = []khttp.ServerOption{
		khttp.Middleware(
			recovery.Recovery(),
			// The auditor sits outside authentication so a rejected call is
			// recorded with its reason instead of vanishing.
			auditor.Middleware(),
			middleware.Auth(authUC),
			validate.Validator(func(req any) error {
				if msg, ok := req.(proto.Message); ok {
					if err := fieldbehavior.ValidateRequiredFields(msg); err != nil {
						return err
					}
				}
				return nil
			}),
		),
	}
	// Every HTTP filter has to be collected into a single list: the kratos
	// option assigns rather than appends, so a second Filter call would
	// silently discard the first one.
	opts = append(opts, khttp.Filter(
		middleware.RequestID(),
		// CORS is governed by web.cors_origins, not by whether this process
		// also serves the front end: a deployment that hosts the SPA elsewhere
		// still needs the headers.
		middleware.CORS(
			webCfg.GetCorsOrigins(),
			webCfg.GetCorsMethods(),
			webCfg.GetCorsHeaders(),
			webCfg.GetCorsAllowCredentials(),
			600,
		),
		// Bound the size of a JSON body. Binary routes are left alone on
		// purpose: their size is governed by upload.max_inline_size and
		// upload.max_chunk_size, and a single chunk may legitimately be
		// gigabytes.
		maxJSONBody(c.GetHttp().GetMaxBodySize()),
	))
	// The API speaks the canonical protobuf JSON mapping so the generated
	// OpenAPI document describes the wire format exactly.
	opts = append(opts,
		khttp.RequestQueryDecoder(ProtoJSONQueryDecoder),
		khttp.RequestDecoder(ProtoJSONRequestDecoder),
		khttp.ResponseEncoder(ProtoJSONResponseEncoder),
	)
	if c.GetHttp().GetNetwork() != "" {
		opts = append(opts, khttp.Network(c.GetHttp().GetNetwork()))
	}
	if c.GetHttp().GetAddr() != "" {
		opts = append(opts, khttp.Address(c.GetHttp().GetAddr()))
	}
	if c.GetHttp().GetTimeout() != nil {
		opts = append(opts, khttp.Timeout(c.GetHttp().GetTimeout().AsDuration()))
	}
	srv := khttp.NewServer(opts...)

	v1.RegisterAuthServiceHTTPServer(srv, auth)
	v1.RegisterUserServiceHTTPServer(srv, user)
	v1.RegisterNodeServiceHTTPServer(srv, node)
	v1.RegisterFileServiceHTTPServer(srv, file)
	v1.RegisterShareServiceHTTPServer(srv, share)
	v1.RegisterAuditServiceHTTPServer(srv, audit)
	v1.RegisterSystemServiceHTTPServer(srv, system)

	registerRawRoutes(srv, stream)
	registerStaticRoutes(srv, webCfg)
	return srv
}

// defaultMaxJSONBody is the JSON body limit used when the configuration does
// not set one. Every JSON endpoint in this API carries metadata or a small
// inline payload, never a whole file.
const defaultMaxJSONBody = 32 << 20

// maxJSONBody caps the body of a JSON request so a single call cannot make the
// server buffer an unbounded amount of data. Requests without a JSON content
// type stream straight through and are bounded by the upload limits instead.
func maxJSONBody(configured int64) khttp.FilterFunc {
	limit := configured
	if limit <= 0 {
		limit = defaultMaxJSONBody
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && strings.Contains(r.Header.Get("Content-Type"), "json") {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// registerRawRoutes adds the binary endpoints that cannot be expressed through
// the proto JSON codec. They authorise themselves, so they live outside the
// generated middleware chain on purpose.
func registerRawRoutes(srv *khttp.Server, stream *StreamHandler) {
	if stream == nil {
		return
	}
	r := srv.Route("/", rawRecovery)
	r.GET("/v1/files/{node_id}/content", stream.Content)
	r.GET("/v1/files/{node_id}/archive", stream.Archive)
	r.PUT("/v1/files/uploads/{upload_id}/parts/{part_number}/raw", stream.UploadRawChunk)
}

// registerStaticRoutes publishes the API documentation and, when it is
// enabled, the front end. They are registered last so the API always wins, and
// the catch-all prefix only sees what nothing else matched.
func registerStaticRoutes(srv *khttp.Server, webCfg *conf.Web) {
	srv.HandlePrefix("/docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir("./docs"))))
	if !webCfg.GetEnabled() {
		return
	}
	prefix := webCfg.GetPathPrefix()
	if prefix == "" {
		prefix = "/"
	}
	srv.HandlePrefix(prefix, NewWebHandler(webCfg))
}

// rawRecovery converts a panic in a raw handler into a 500 instead of taking
// the process down. The generated routes get the same protection from the
// recovery middleware; the raw ones are registered outside that chain.
func rawRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
