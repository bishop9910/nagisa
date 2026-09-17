package biz

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Share is a capability link that publishes a node to callers without an
// account.
type Share struct {
	ID            uuid.UUID
	Token         string
	NodeID        uuid.UUID
	OwnerID       uuid.UUID
	Name          string
	Description   string
	Permissions   PermMask
	PasswordHash  string
	PasswordHint  string
	ExpiresAt     *time.Time
	MaxDownloads  int64
	DownloadCount int64
	ViewCount     int64
	Status        ShareStatus
	CreatedBy     uuid.UUID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Expired reports whether the link has passed its expiry.
func (s *Share) Expired(now time.Time) bool {
	return s != nil && s.ExpiresAt != nil && now.After(*s.ExpiresAt)
}

// Exhausted reports whether the download budget has been used up.
func (s *Share) Exhausted() bool {
	return s != nil && s.MaxDownloads > 0 && s.DownloadCount >= s.MaxDownloads
}

// Visible reports whether the link can still be redeemed.
func (s *Share) Visible(now time.Time) bool {
	return s != nil && s.Status == ShareStatusActive && !s.Expired(now) && !s.Exhausted()
}

// ShareRepo persists share links.
type ShareRepo interface {
	CreateShare(context.Context, *Share) (*Share, error)
	FindShareByID(context.Context, uuid.UUID) (*Share, error)
	FindShareByToken(context.Context, string) (*Share, error)
	ListShares(context.Context, ...ListOption) ([]*Share, error)
	CountShares(context.Context, ...ListOption) (int64, error)
	ListSharesByNode(context.Context, uuid.UUID, ...ListOption) ([]*Share, error)
	UpdateShare(context.Context, *Share) (*Share, error)
	DeleteShare(context.Context, uuid.UUID) error
	AddShareView(context.Context, uuid.UUID) error
	AddShareDownload(context.Context, uuid.UUID) error
	CountSharesByNodes(context.Context, []uuid.UUID) (map[uuid.UUID]int64, error)
	ListExpiredShares(context.Context, time.Time, int) ([]*Share, error)
}

// ShareUsecaseOptions carries the share configuration.
type ShareUsecaseOptions struct {
	AllowPublic     bool
	PublicBaseURL   string
	SharePathPrefix string
	AccessTokenTTL  time.Duration
}

// ShareUsecase implements share link management and public access.
type ShareUsecase struct {
	nodes       *NodeUsecase
	repo        ShareRepo
	tx          TxManager
	hash        PasswordVerifier
	hasher      Hasher
	store       ObjectStore
	signer      URLSigner
	allowPublic bool
	baseURL     string
	sharePrefix string
	accessTTL   time.Duration
}

// Hasher hashes a clear text password.
type Hasher interface {
	Hash(password string) (string, error)
}

// NewShareUsecase returns a share usecase.
func NewShareUsecase(nodes *NodeUsecase, repo ShareRepo, tx TxManager, hash PasswordVerifier, hasher Hasher, store ObjectStore, signer URLSigner, opts ShareUsecaseOptions) *ShareUsecase {
	if opts.SharePathPrefix == "" {
		opts.SharePathPrefix = "/s"
	}
	if opts.AccessTokenTTL <= 0 {
		opts.AccessTokenTTL = 2 * time.Hour
	}
	return &ShareUsecase{
		nodes:       nodes,
		repo:        repo,
		tx:          tx,
		hash:        hash,
		hasher:      hasher,
		store:       store,
		signer:      signer,
		allowPublic: opts.AllowPublic,
		baseURL:     strings.TrimRight(opts.PublicBaseURL, "/"),
		sharePrefix: opts.SharePathPrefix,
		accessTTL:   opts.AccessTokenTTL,
	}
}

// AccessTTL is the lifetime of a share access token.
func (uc *ShareUsecase) AccessTTL() time.Duration { return uc.accessTTL }

// URL builds the public URL of a share token.
func (uc *ShareUsecase) URL(token string) string {
	return uc.baseURL + uc.sharePrefix + "/" + token
}

// CreateShareInput describes a new share link.
type CreateShareInput struct {
	NodeID       uuid.UUID
	Name         string
	Description  string
	Permissions  PermMask
	PasswordHash string
	PasswordHint string
	ExpiresAt    *time.Time
	MaxDownloads int64
	Token        string
}

// Create publishes a node.
func (uc *ShareUsecase) Create(ctx context.Context, in CreateShareInput) (*Share, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	if !uc.allowPublic {
		return nil, ErrUnsupported
	}
	if in.NodeID == uuid.Nil {
		return nil, InvalidArgument("node_id is required")
	}
	node, access, err := uc.nodes.RequireAccess(ctx, caller, in.NodeID, PermShare)
	if err != nil {
		return nil, err
	}
	perms := in.Permissions.And(PermShareScope)
	if perms.IsZero() {
		perms = PermView | PermDownload
	}
	if perms.Has(PermDownload) && !access.Perms.Has(PermDownload) {
		return nil, ErrPermissionDenied
	}
	if perms.Has(PermUpload) && (!node.IsFolder() || !access.Perms.Has(PermUpload)) {
		return nil, ErrPermissionDenied
	}
	if !caller.Perms.Has(perms) {
		return nil, ErrPermissionDenied
	}
	token := strings.TrimSpace(in.Token)
	if token != "" && !validShareToken(token) {
		return nil, InvalidArgument("token must be 8-64 characters of [A-Za-z0-9_-]")
	}
	if token == "" {
		token, err = randomToken()
		if err != nil {
			return nil, err
		}
	}
	share := &Share{
		ID:           NewID(),
		Token:        token,
		NodeID:       node.ID,
		OwnerID:      caller.UserID,
		Name:         strings.TrimSpace(in.Name),
		Description:  in.Description,
		Permissions:  perms,
		PasswordHash: in.PasswordHash,
		PasswordHint: in.PasswordHint,
		ExpiresAt:    in.ExpiresAt,
		MaxDownloads: in.MaxDownloads,
		Status:       ShareStatusActive,
		CreatedBy:    caller.UserID,
	}
	if share.Name == "" {
		share.Name = node.Name
	}
	var created *Share
	err = uc.tx.WithTx(ctx, func(ctx context.Context) error {
		existing, err := uc.repo.FindShareByToken(ctx, token)
		if err == nil && existing != nil {
			return ErrAlreadyExists
		}
		if err != nil && err != ErrNotFound {
			return err
		}
		created, err = uc.repo.CreateShare(ctx, share)
		return err
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Get returns one share.
func (uc *ShareUsecase) Get(ctx context.Context, id uuid.UUID) (*Share, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	share, err := uc.repo.FindShareByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if share.OwnerID != caller.UserID && !caller.Has(PermUserManage) {
		return nil, ErrPermissionDenied
	}
	return share, nil
}

// List returns share links the caller may see.
func (uc *ShareUsecase) List(ctx context.Context, filter string, opts ...ListOption) ([]*Share, int64, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, 0, ErrUnauthenticated
	}
	shares, err := uc.repo.ListShares(ctx, opts...)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.repo.CountShares(ctx, opts...)
	if err != nil {
		return nil, 0, err
	}
	if caller.Has(PermUserManage) {
		return shares, total, nil
	}
	owned := make([]*Share, 0, len(shares))
	for _, s := range shares {
		if s.OwnerID == caller.UserID {
			owned = append(owned, s)
		}
	}
	return owned, total, nil
}

// ListByNode returns the shares attached to a node.
func (uc *ShareUsecase) ListByNode(ctx context.Context, nodeID uuid.UUID, opts ...ListOption) ([]*Share, error) {
	caller := CallerFromContext(ctx)
	if _, _, err := uc.nodes.RequireAccess(ctx, caller, nodeID, PermShare); err != nil {
		return nil, err
	}
	return uc.repo.ListSharesByNode(ctx, nodeID, opts...)
}

// UpdateShareInput describes a share update.
type UpdateShareInput struct {
	ID            uuid.UUID
	Name          *string
	Description   *string
	Permissions   *PermMask
	PasswordHash  *string
	PasswordHint  *string
	ClearPassword bool
	ExpiresAt     *time.Time
	ClearExpiry   bool
	MaxDownloads  *int64
	Status        *ShareStatus
}

// Update changes an existing share link.
func (uc *ShareUsecase) Update(ctx context.Context, in UpdateShareInput) (*Share, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	var out *Share
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		share, err := uc.repo.FindShareByID(ctx, in.ID)
		if err != nil {
			return err
		}
		if share.OwnerID != caller.UserID && !caller.Has(PermUserManage) {
			return ErrPermissionDenied
		}
		if in.Name != nil {
			share.Name = *in.Name
		}
		if in.Description != nil {
			share.Description = *in.Description
		}
		if in.Permissions != nil {
			perms := in.Permissions.And(PermShareScope)
			if !caller.Perms.Has(perms) {
				return ErrPermissionDenied
			}
			share.Permissions = perms
		}
		if in.PasswordHash != nil {
			share.PasswordHash = *in.PasswordHash
		}
		if in.PasswordHint != nil {
			share.PasswordHint = *in.PasswordHint
		}
		if in.ClearPassword {
			share.PasswordHash, share.PasswordHint = "", ""
		}
		if in.ClearExpiry {
			share.ExpiresAt = nil
		} else if in.ExpiresAt != nil {
			share.ExpiresAt = in.ExpiresAt
		}
		if in.MaxDownloads != nil {
			share.MaxDownloads = *in.MaxDownloads
		}
		if in.Status != nil {
			share.Status = *in.Status
		}
		out, err = uc.repo.UpdateShare(ctx, share)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Delete revokes a share link.
func (uc *ShareUsecase) Delete(ctx context.Context, id uuid.UUID) error {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return ErrUnauthenticated
	}
	return uc.tx.WithTx(ctx, func(ctx context.Context) error {
		share, err := uc.repo.FindShareByID(ctx, id)
		if err != nil {
			return err
		}
		if share.OwnerID != caller.UserID && !caller.Has(PermUserManage) {
			return ErrPermissionDenied
		}
		return uc.repo.DeleteShare(ctx, id)
	})
}

// Resolve loads a share by token and checks that it is still redeemable. It is
// the entry point of every public share endpoint.
func (uc *ShareUsecase) Resolve(ctx context.Context, token string) (*Share, error) {
	if !uc.allowPublic {
		return nil, ErrUnsupported
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrInvalidArgument
	}
	share, err := uc.repo.FindShareByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	switch {
	case share.Status == ShareStatusRevoked:
		return nil, ErrNotFound
	case share.Expired(now) || share.Status == ShareStatusExpired:
		return nil, ErrNotFound
	case share.Exhausted():
		return nil, ErrResourceExhausted
	}
	return share, nil
}

// VerifyPassword checks a share password.
func (uc *ShareUsecase) VerifyPassword(share *Share, password string) error {
	if share == nil {
		return ErrNotFound
	}
	if share.PasswordHash == "" {
		return nil
	}
	if password == "" {
		return ErrNodeLocked
	}
	if uc.hash == nil || uc.hash.Verify(share.PasswordHash, password) != nil {
		return ErrPermissionDenied
	}
	return nil
}

// HashPassword hashes a link password.
func (uc *ShareUsecase) HashPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	return uc.hasher.Hash(password)
}

// Node returns the shared node.
func (uc *ShareUsecase) Node(ctx context.Context, share *Share) (*Node, error) {
	return uc.nodes.repo.FindNodeByID(ctx, share.NodeID)
}

// ResolveNode resolves a node inside a share, ensuring it belongs to the
// shared subtree.
func (uc *ShareUsecase) ResolveNode(ctx context.Context, share *Share, nodeID uuid.UUID) (*Node, error) {
	root, err := uc.Node(ctx, share)
	if err != nil {
		return nil, err
	}
	if nodeID == uuid.Nil || nodeID == root.ID {
		return root, nil
	}
	node, err := uc.nodes.repo.FindNodeByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node.Status != NodeStatusActive {
		return nil, ErrNotFound
	}
	if !strings.HasPrefix(node.Path, root.Path) {
		return nil, ErrPermissionDenied
	}
	return node, nil
}

// Children lists the first level below a node inside a share.
func (uc *ShareUsecase) Children(ctx context.Context, share *Share, folder *Node, opts ...ListOption) ([]*Node, int64, error) {
	if folder.Kind != NodeKindFolder {
		return nil, 0, nil
	}
	parentID := folder.ID
	query := NodeQuery{ParentID: &parentID}
	nodes, err := uc.nodes.repo.ListNodes(ctx, query, opts...)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.nodes.repo.CountNodes(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	return nodes, total, nil
}

// CountsByNodes returns the number of active share links per node, used to
// decorate node replies.
func (uc *ShareUsecase) CountsByNodes(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
	return uc.repo.CountSharesByNodes(ctx, ids)
}

// MarkViewed records one successful resolution of a link.
func (uc *ShareUsecase) MarkViewed(ctx context.Context, id uuid.UUID) error {
	return uc.tx.WithTx(ctx, func(ctx context.Context) error {
		return uc.repo.AddShareView(ctx, id)
	})
}

// MarkDownloaded records one served download against the link budget.
func (uc *ShareUsecase) MarkDownloaded(ctx context.Context, id uuid.UUID) error {
	return uc.tx.WithTx(ctx, func(ctx context.Context) error {
		return uc.repo.AddShareDownload(ctx, id)
	})
}

// Grant is the permission set a caller holds on a share.
func (uc *ShareUsecase) Grant(share *Share, unlocked bool) NodeAccess {
	perms := share.Permissions.And(PermShareScope)
	if share.PasswordHash != "" && !unlocked {
		perms = perms.And(PermView)
	}
	return NodeAccess{Perms: perms, Visibility: VisibilityPublic, Locked: share.PasswordHash != "" && !unlocked}
}

// Download grants a signed URL for a node published through a share. A folder
// resolves to a signed archive URL, which is what a visitor expects when the
// shared node is a directory.
func (uc *ShareUsecase) Download(ctx context.Context, share *Share, node *Node, unlocked bool, expiresIn int32, inline bool, fileName string) (*SignedURL, error) {
	grant := uc.Grant(share, unlocked)
	if !grant.Perms.Has(PermDownload) {
		return nil, ErrPermissionDenied
	}
	if node.Kind != NodeKindFile || node.StorageKey == "" {
		return nil, ErrUnsupported
	}
	if fileName == "" {
		fileName = node.Name
	}
	ttl := uc.presignTTL(expiresIn)
	// Same rule as a signed-in download: a presigned object storage URL is only
	// usable from the browser when the deployment declared a public endpoint,
	// otherwise it would send a visitor on another device to its own loopback.
	if !browserCanReachStorage(uc.store) {
		return streamedContentURL(uc.signer, node, share.ID.String(), ttl, inline, fileName)
	}
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	req, err := uc.store.PresignGetObject(ctx, node.StorageKey, fileName, node.MimeType, disposition, ttl)
	if err != nil {
		return nil, err
	}
	return &SignedURL{
		URL: req.URL, Method: req.Method, Headers: req.Headers, ExpiresAt: req.ExpiresAt,
		NodeID: node.ID, Size: node.Size, FileName: fileName, MimeType: node.MimeType,
	}, nil
}

// presignTTL clamps a requested lifetime to the configured maximum.
func (uc *ShareUsecase) presignTTL(requested int32) time.Duration {
	ttl := 30 * time.Minute
	if requested > 0 {
		ttl = time.Duration(requested) * time.Second
	}
	if ttl > 4*time.Hour {
		ttl = 4 * time.Hour
	}
	return ttl
}

// validShareToken keeps share tokens URL safe and long enough to be
// unguessable.
func validShareToken(token string) bool {
	if len(token) < 8 || len(token) > 64 {
		return false
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// randomToken returns an unguessable URL safe token.
func randomToken() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
