package biz

import (
	"context"
	"strings"
	"time"

	"nagisa/internal/pkg/crypt"

	"github.com/google/uuid"
)

// TokenPair is the credential pair returned by a successful login.
type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	TokenType        string
	ExpiresIn        int32
	RefreshExpiresIn int32
}

// AuthConfig is the public pre-login parameter set.
type AuthConfig struct {
	PasswordKeyID        string
	PasswordEncoding     string
	PasswordPublicKey    string
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	PlainPasswordAllowed bool
	GuestLoginEnabled    bool
}

// AuthUsecaseOptions carries the credential policy taken from configuration.
type AuthUsecaseOptions struct {
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	NodeTokenTTL       time.Duration
	PlainPasswordAllow bool
	MinPasswordLength  int
	// GuestUsername names the built-in read-only account, and GuestAutoLogin
	// lets a client open a session for it without a password.
	GuestUsername  string
	GuestAutoLogin bool
}

// AuthUsecase implements credential exchange, session lifecycle and the token
// issuing used by the transport middleware.
type AuthUsecase struct {
	users        UserRepo
	tx           TxManager
	hasher       Hasher
	verifier     PasswordVerifier
	issuer       *crypt.Issuer
	box          *crypt.KeyBox
	accessTTL    time.Duration
	refreshTTL   time.Duration
	nodeTTL      time.Duration
	plainAllowed bool
	minPassword  int
	guestName    string
	guestAuto    bool
}

// NewAuthUsecase returns an auth usecase.
func NewAuthUsecase(users UserRepo, tx TxManager, hasher Hasher, verifier PasswordVerifier, issuer *crypt.Issuer, box *crypt.KeyBox, opts AuthUsecaseOptions) *AuthUsecase {
	if opts.AccessTokenTTL <= 0 {
		opts.AccessTokenTTL = 2 * time.Hour
	}
	if opts.RefreshTokenTTL <= 0 {
		opts.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	if opts.NodeTokenTTL <= 0 {
		opts.NodeTokenTTL = 30 * time.Minute
	}
	if opts.MinPasswordLength <= 0 {
		opts.MinPasswordLength = 8
	}
	if opts.GuestUsername == "" {
		opts.GuestUsername = "guest"
	}
	return &AuthUsecase{
		users: users, tx: tx, hasher: hasher, verifier: verifier,
		issuer: issuer, box: box,
		accessTTL: opts.AccessTokenTTL, refreshTTL: opts.RefreshTokenTTL,
		nodeTTL: opts.NodeTokenTTL, plainAllowed: opts.PlainPasswordAllow,
		minPassword: opts.MinPasswordLength,
		guestName:   opts.GuestUsername, guestAuto: opts.GuestAutoLogin,
	}
}

// Config returns the parameters a client needs before logging in.
func (uc *AuthUsecase) Config() AuthConfig {
	return AuthConfig{
		PasswordKeyID:        uc.box.KeyID(),
		PasswordEncoding:     crypt.PasswordEncoding,
		PasswordPublicKey:    uc.box.PublicKeyPEM(),
		AccessTokenTTL:       uc.accessTTL,
		RefreshTokenTTL:      uc.refreshTTL,
		PlainPasswordAllowed: uc.plainAllowed,
		GuestLoginEnabled:    uc.guestAuto,
	}
}

// AccessTokenTTL is the lifetime of an access token.
func (uc *AuthUsecase) AccessTokenTTL() time.Duration { return uc.accessTTL }

// RefreshTokenTTL is the lifetime of a refresh token.
func (uc *AuthUsecase) RefreshTokenTTL() time.Duration { return uc.refreshTTL }

// NodeTokenTTL is the lifetime of a folder unlock token.
func (uc *AuthUsecase) NodeTokenTTL() time.Duration { return uc.nodeTTL }

// MinPasswordLength is the configured minimum password length.
func (uc *AuthUsecase) MinPasswordLength() int { return uc.minPassword }

// DecodePassword turns a client supplied password field into clear text.
func (uc *AuthUsecase) DecodePassword(field string) (string, error) {
	if field == "" {
		return "", ErrInvalidArgument
	}
	plain, err := uc.box.Decode(field, uc.plainAllowed)
	if err != nil {
		return "", ErrInvalidArgument
	}
	return plain, nil
}

// ValidatePassword applies the configured password policy.
func (uc *AuthUsecase) ValidatePassword(plain string) error {
	if len([]rune(plain)) < uc.minPassword {
		return ErrInvalidArgument
	}
	return nil
}

// HashPassword returns a storable hash and enforces the policy.
func (uc *AuthUsecase) HashPassword(plain string) (string, error) {
	if err := uc.ValidatePassword(plain); err != nil {
		return "", err
	}
	return uc.hasher.Hash(plain)
}

// LoginInput carries the fields of a login attempt.
type LoginInput struct {
	Username  string
	Password  string
	IP        string
	UserAgent string
	Remember  bool
}

// Login authenticates an account and opens a session.
func (uc *AuthUsecase) Login(ctx context.Context, in LoginInput) (*TokenPair, *User, error) {
	username := strings.TrimSpace(in.Username)
	if username == "" || in.Password == "" {
		return nil, nil, ErrInvalidArgument
	}
	plain, err := uc.DecodePassword(in.Password)
	if err != nil {
		return nil, nil, err
	}
	user, err := uc.users.FindUserByUsername(ctx, username)
	if err != nil {
		return nil, nil, ErrUnauthenticated
	}
	if uc.verifier.Verify(user.PasswordHash, plain) != nil {
		return nil, nil, ErrUnauthenticated
	}
	if user.Status != UserStatusActive {
		return nil, nil, ErrAccountDisabled
	}
	ttl := uc.refreshTTL
	if in.Remember {
		ttl = uc.refreshTTL * 2
	}
	pair, err := uc.openSession(ctx, user, in.IP, in.UserAgent, ttl)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	user.LastLoginAt = &now
	if err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		_, err := uc.users.UpdateUser(ctx, user)
		return err
	}); err != nil {
		return nil, nil, err
	}
	return pair, user, nil
}

// GuestLogin opens a read-only session for the built-in guest account without a
// password. The reply carries an access token only: a guest has no refresh
// session to rotate, which keeps every anonymous visitor from leaving a row
// behind, so a client simply asks again when the token expires.
func (uc *AuthUsecase) GuestLogin(ctx context.Context) (*TokenPair, *User, error) {
	if !uc.guestAuto {
		return nil, nil, ErrUnsupported
	}
	user, err := uc.users.FindUserByUsername(ctx, uc.guestName)
	if err != nil {
		return nil, nil, ErrUnauthenticated
	}
	if user.Status != UserStatusActive {
		return nil, nil, ErrAccountDisabled
	}
	access, err := uc.issueAccess(user)
	if err != nil {
		return nil, nil, err
	}
	user.Locked = true
	return &TokenPair{
		AccessToken: access,
		TokenType:   "Bearer",
		ExpiresIn:   int32(uc.accessTTL.Seconds()),
	}, user, nil
}

// openSession mints an access token and records a refresh session.
func (uc *AuthUsecase) openSession(ctx context.Context, user *User, ip, ua string, refreshTTL time.Duration) (*TokenPair, error) {
	access, err := uc.issueAccess(user)
	if err != nil {
		return nil, err
	}
	raw, err := randomToken()
	if err != nil {
		return nil, err
	}
	session := &Session{
		UserID:    user.ID,
		TokenHash: crypt.HashToken(raw),
		IP:        ip,
		UserAgent: ua,
		ExpiresAt: time.Now().Add(refreshTTL),
	}
	if _, err := uc.users.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:      access,
		RefreshToken:     raw,
		TokenType:        "Bearer",
		ExpiresIn:        int32(uc.accessTTL.Seconds()),
		RefreshExpiresIn: int32(refreshTTL.Seconds()),
	}, nil
}

// issueAccess signs an access token for an account.
func (uc *AuthUsecase) issueAccess(user *User) (string, error) {
	return uc.issuer.Issue(crypt.Claims{
		Type:     crypt.TokenTypeAccess,
		Username: user.Username,
		Role:     int32(user.Role),
		Rank:     user.Rank,
		Perms:    int64(user.Permissions),
	}, user.ID.String(), uc.accessTTL)
}

// Refresh rotates a refresh token into a new pair.
func (uc *AuthUsecase) Refresh(ctx context.Context, refreshToken, ip, ua string) (*TokenPair, *User, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, nil, ErrInvalidArgument
	}
	var pair *TokenPair
	var user *User
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		session, err := uc.users.FindSessionByHash(ctx, crypt.HashToken(refreshToken))
		if err != nil {
			return ErrUnauthenticated
		}
		now := time.Now()
		if !session.Active(now) {
			return ErrUnauthenticated
		}
		target, err := uc.users.FindUserByID(ctx, session.UserID)
		if err != nil {
			return ErrUnauthenticated
		}
		if target.Status != UserStatusActive {
			return ErrAccountDisabled
		}
		// Rotate: the presented token stops working the moment it is used.
		if err := uc.users.RevokeSession(ctx, session.ID); err != nil {
			return err
		}
		pair, err = uc.openSession(ctx, target, ip, ua, time.Until(session.ExpiresAt))
		if err != nil {
			return err
		}
		user = target
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return pair, user, nil
}

// Logout revokes one refresh token, or every session of the caller.
func (uc *AuthUsecase) Logout(ctx context.Context, refreshToken string, all bool) error {
	caller := CallerFromContext(ctx)
	if all || strings.TrimSpace(refreshToken) == "" {
		if !caller.IsAuthenticated() {
			return ErrUnauthenticated
		}
		return uc.tx.WithTx(ctx, func(ctx context.Context) error {
			return uc.users.RevokeUserSessions(ctx, caller.UserID)
		})
	}
	session, err := uc.users.FindSessionByHash(ctx, crypt.HashToken(refreshToken))
	if err != nil {
		return ErrUnauthenticated
	}
	if caller.IsAuthenticated() && session.UserID != caller.UserID {
		return ErrPermissionDenied
	}
	return uc.tx.WithTx(ctx, func(ctx context.Context) error {
		return uc.users.RevokeSession(ctx, session.ID)
	})
}

// ChangePassword replaces the caller's own password.
func (uc *AuthUsecase) ChangePassword(ctx context.Context, currentField, newField string) error {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return ErrUnauthenticated
	}
	current, err := uc.DecodePassword(currentField)
	if err != nil {
		return err
	}
	next, err := uc.DecodePassword(newField)
	if err != nil {
		return err
	}
	if err := uc.ValidatePassword(next); err != nil {
		return err
	}
	hash, err := uc.hasher.Hash(next)
	if err != nil {
		return err
	}
	return uc.tx.WithTx(ctx, func(ctx context.Context) error {
		user, err := uc.users.FindUserByID(ctx, caller.UserID)
		if err != nil {
			return err
		}
		if uc.verifier.Verify(user.PasswordHash, current) != nil {
			return ErrUnauthenticated
		}
		user.PasswordHash = hash
		user.MustChangePassword = false
		if _, err := uc.users.UpdateUser(ctx, user); err != nil {
			return err
		}
		return uc.users.RevokeUserSessions(ctx, user.ID)
	})
}

// Sessions lists the caller's refresh sessions.
func (uc *AuthUsecase) Sessions(ctx context.Context, includeInactive bool, opts ...ListOption) ([]*Session, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	return uc.users.ListSessions(ctx, caller.UserID, includeInactive, opts...)
}

// RevokeSession revokes one of the caller's sessions.
func (uc *AuthUsecase) RevokeSession(ctx context.Context, id uuid.UUID) error {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return ErrUnauthenticated
	}
	session, err := uc.users.FindSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if session.UserID != caller.UserID {
		return ErrPermissionDenied
	}
	return uc.tx.WithTx(ctx, func(ctx context.Context) error {
		return uc.users.RevokeSession(ctx, id)
	})
}

// Authenticate validates an access token and builds the caller identity.
func (uc *AuthUsecase) Authenticate(ctx context.Context, token string) (*Caller, error) {
	claims, err := uc.issuer.Verify(token)
	if err != nil || claims.Type != crypt.TokenTypeAccess {
		return nil, ErrUnauthenticated
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	user, err := uc.users.FindUserByID(ctx, id)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	if user.Status != UserStatusActive {
		return nil, ErrAccountDisabled
	}
	return &Caller{
		UserID:   user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Role:     user.Role,
		Rank:     user.Rank,
		Perms:    user.Permissions,
		Status:   user.Status,
		Unlocked: map[uuid.UUID]bool{},
	}, nil
}

// IssueNodeToken returns a token that proves knowledge of a folder password
// for the supplied nodes and their subtrees.
func (uc *AuthUsecase) IssueNodeToken(nodes []uuid.UUID) (string, time.Time, error) {
	ids := make([]string, 0, len(nodes))
	for _, id := range nodes {
		if id != uuid.Nil {
			ids = append(ids, id.String())
		}
	}
	expires := time.Now().Add(uc.nodeTTL)
	token, err := uc.issuer.Issue(crypt.Claims{Type: crypt.TokenTypeNode, Nodes: ids}, "", uc.nodeTTL)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// ParseNodeToken returns the node ids a folder unlock token covers.
func (uc *AuthUsecase) ParseNodeToken(token string) ([]uuid.UUID, error) {
	claims, err := uc.issuer.Verify(token)
	if err != nil || claims.Type != crypt.TokenTypeNode {
		return nil, ErrUnauthenticated
	}
	out := make([]uuid.UUID, 0, len(claims.Nodes))
	for _, raw := range claims.Nodes {
		if id, err := uuid.Parse(raw); err == nil {
			out = append(out, id)
		}
	}
	return out, nil
}

// IssueShareToken returns a token that carries a share id for the duration of
// a browsing session.
func (uc *AuthUsecase) IssueShareToken(shareID uuid.UUID, ttl time.Duration) (string, error) {
	return uc.issuer.Issue(crypt.Claims{Type: crypt.TokenTypeShare, ShareID: shareID.String()}, shareID.String(), ttl)
}

// ParseShareToken returns the share id carried by an access token.
func (uc *AuthUsecase) ParseShareToken(token string) (uuid.UUID, error) {
	claims, err := uc.issuer.Verify(token)
	if err != nil || claims.Type != crypt.TokenTypeShare {
		return uuid.Nil, ErrUnauthenticated
	}
	return uuid.Parse(claims.Subject)
}
