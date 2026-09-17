package service

import (
	"context"
	"strings"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"

	"github.com/go-kratos/kratos/v3/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AuthService serves credential exchange and session lifecycle requests.
type AuthService struct {
	v1.UnimplementedAuthServiceServer

	uc    *biz.AuthUsecase
	users *biz.UserUsecase
}

// NewAuthService returns an authentication service adapter.
func NewAuthService(uc *biz.AuthUsecase, users *biz.UserUsecase) *AuthService {
	return &AuthService{uc: uc, users: users}
}

// GetAuthConfig returns the pre-login parameter set.
func (s *AuthService) GetAuthConfig(ctx context.Context, _ *v1.GetAuthConfigRequest) (*v1.AuthConfig, error) {
	cfg := s.uc.Config()
	return &v1.AuthConfig{
		PasswordKeyId:          cfg.PasswordKeyID,
		PasswordEncoding:       cfg.PasswordEncoding,
		PasswordPublicKey:      cfg.PasswordPublicKey,
		AccessTokenTtlSeconds:  int32(cfg.AccessTokenTTL.Seconds()),
		RefreshTokenTtlSeconds: int32(cfg.RefreshTokenTTL.Seconds()),
		PlainPasswordAllowed:   cfg.PlainPasswordAllowed,
		GuestLoginEnabled:      cfg.GuestLoginEnabled,
		MinPasswordLength:      int32(cfg.MinPasswordLength),
		ServerTime:             timestamppb.Now(),
	}, nil
}

// GuestLogin opens a read-only session for the built-in guest account without a
// password, which is what lets a client show readable content straight away.
func (s *AuthService) GuestLogin(ctx context.Context, _ *v1.GuestLoginRequest) (*v1.LoginReply, error) {
	pair, user, err := s.uc.GuestLogin(ctx)
	if err != nil {
		return nil, err
	}
	return loginReply(pair, user, biz.CallerFromContext(ctx)), nil
}

// Login exchanges an account name and an encoded password for a token pair.
func (s *AuthService) Login(ctx context.Context, req *v1.LoginRequest) (*v1.LoginReply, error) {
	if strings.TrimSpace(req.GetUsername()) == "" || req.GetPassword() == "" {
		return nil, biz.ErrInvalidArgument
	}
	ip, ua := clientInfo(ctx)
	if hint := req.GetIp(); hint != "" {
		ip = hint
	}
	if hint := req.GetUserAgent(); hint != "" {
		ua = hint
	}
	pair, user, err := s.uc.Login(ctx, biz.LoginInput{
		Username:  req.GetUsername(),
		Password:  req.GetPassword(),
		IP:        ip,
		UserAgent: ua,
		Remember:  req.GetRememberMe(),
	})
	if err != nil {
		return nil, err
	}
	return loginReply(pair, user, biz.CallerFromContext(ctx)), nil
}

// RefreshToken rotates a refresh token into a fresh token pair.
func (s *AuthService) RefreshToken(ctx context.Context, req *v1.RefreshTokenRequest) (*v1.LoginReply, error) {
	if strings.TrimSpace(req.GetRefreshToken()) == "" {
		return nil, biz.ErrInvalidArgument
	}
	ip, ua := clientInfo(ctx)
	pair, user, err := s.uc.Refresh(ctx, req.GetRefreshToken(), ip, ua)
	if err != nil {
		return nil, err
	}
	return loginReply(pair, user, biz.CallerFromContext(ctx)), nil
}

// Logout revokes the presented refresh token, or every session of the account.
func (s *AuthService) Logout(ctx context.Context, req *v1.LogoutRequest) (*emptypb.Empty, error) {
	if err := s.uc.Logout(ctx, req.GetRefreshToken(), req.GetRevokeAll()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// GetCurrentUser returns the account behind the presented access token.
func (s *AuthService) GetCurrentUser(ctx context.Context, _ *v1.GetCurrentUserRequest) (*v1.User, error) {
	caller := biz.CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, biz.ErrUnauthenticated
	}
	user, err := s.users.GetUser(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	return decorateUser(convertUser(user), caller, user), nil
}

// ChangePassword replaces the caller's own password.
func (s *AuthService) ChangePassword(ctx context.Context, req *v1.ChangePasswordRequest) (*emptypb.Empty, error) {
	if req.GetCurrentPassword() == "" || req.GetNewPassword() == "" {
		return nil, biz.ErrInvalidArgument
	}
	if err := s.uc.ChangePassword(ctx, req.GetCurrentPassword(), req.GetNewPassword()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ListSessions returns a page of the caller's refresh sessions.
func (s *AuthService) ListSessions(ctx context.Context, req *v1.ListSessionsRequest) (*v1.SessionSet, error) {
	decl, err := declarations()
	if err != nil {
		return nil, err
	}
	listReq := sessionListRequest{ListSessionsRequest: req}
	opts, size, token, err := parseList(listReq, decl, maxPageSize)
	if err != nil {
		return nil, err
	}
	sessions, err := s.uc.Sessions(ctx, req.GetIncludeInactive(), opts...)
	if err != nil {
		return nil, err
	}
	out := &v1.SessionSet{Sessions: make([]*v1.Session, 0, len(sessions))}
	for _, session := range sessions {
		out.Sessions = append(out.Sessions, convertSession(session))
	}
	out.NextPageToken = nextPageToken(listReq, token, len(sessions), int(size))
	return out, nil
}

// RevokeSession revokes one of the caller's own sessions.
func (s *AuthService) RevokeSession(ctx context.Context, req *v1.RevokeSessionRequest) (*emptypb.Empty, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.uc.RevokeSession(ctx, id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// sessionListRequest adapts the session list request to the AIP list contract.
// The message carries no filter and no order_by field, so both read as empty.
type sessionListRequest struct {
	*v1.ListSessionsRequest
}

func (sessionListRequest) GetFilter() string  { return "" }
func (sessionListRequest) GetOrderBy() string { return "" }

// loginReply renders a token pair together with the authenticated account.
func loginReply(pair *biz.TokenPair, user *biz.User, caller *biz.Caller) *v1.LoginReply {
	return &v1.LoginReply{
		AccessToken:      pair.AccessToken,
		RefreshToken:     pair.RefreshToken,
		TokenType:        pair.TokenType,
		ExpiresIn:        pair.ExpiresIn,
		RefreshExpiresIn: pair.RefreshExpiresIn,
		User:             decorateUser(convertUser(user), caller, user),
	}
}

// clientInfo reads the client address and user agent from the request metadata.
// A proxy reported address wins over the direct peer address.
func clientInfo(ctx context.Context) (string, string) {
	md, ok := metadata.FromServerContext(ctx)
	if !ok {
		return "", ""
	}
	var ip string
	if forwarded := md.Get("x-forwarded-for"); forwarded != "" {
		ip = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	if ip == "" {
		ip = strings.TrimSpace(md.Get("x-real-ip"))
	}
	return ip, strings.TrimSpace(md.Get("user-agent"))
}
