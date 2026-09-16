package service

import (
	"context"
	"errors"
	"strings"
	"time"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"

	"go.einride.tech/aip/filtering"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ShareService implements the share link API. AccessShare, ListShareChildren
// and GetShareDownloadUrl are the anonymous endpoints: they authenticate with
// the share token instead of an account.
type ShareService struct {
	v1.UnimplementedShareServiceServer

	uc    *biz.ShareUsecase
	nodes *biz.NodeUsecase
	auth  *biz.AuthUsecase
}

// NewShareService new a share service.
func NewShareService(uc *biz.ShareUsecase, nodes *biz.NodeUsecase, auth *biz.AuthUsecase) *ShareService {
	return &ShareService{uc: uc, nodes: nodes, auth: auth}
}

// sharePageSize caps one page of share links.
const sharePageSize = 200

// shareDeclarations describes the filterable fields of a share.
func shareDeclarations() (*filtering.Declarations, error) {
	return declarations(
		filtering.DeclareIdent("node_id", filtering.TypeString),
		filtering.DeclareIdent("owner_id", filtering.TypeString),
		filtering.DeclareIdent("name", filtering.TypeString),
		filtering.DeclareIdent("status", filtering.TypeInt),
		filtering.DeclareIdent("token", filtering.TypeString),
		filtering.DeclareIdent("created_at", filtering.TypeTimestamp),
		filtering.DeclareIdent("expires_at", filtering.TypeTimestamp),
	)
}

// shareOrderPaths lists the orderable fields of a share.
var shareOrderPaths = []string{"name", "created_at", "updated_at", "expires_at", "download_count", "view_count"}

// The public share messages paginate but carry no filter field, so each one
// needs a thin adapter to reach the shared AIP list helpers.
type (
	// shareByNodePage adapts ListSharesByNodeRequest, which paginates only.
	shareByNodePage struct{ *v1.ListSharesByNodeRequest }
	// shareAccessPage adapts AccessShareRequest, which paginates only.
	shareAccessPage struct{ *v1.AccessShareRequest }
	// shareChildrenPage adapts ListShareChildrenRequest, which orders but
	// cannot filter.
	shareChildrenPage struct{ *v1.ListShareChildrenRequest }
)

func (shareByNodePage) GetFilter() string  { return "" }
func (shareByNodePage) GetOrderBy() string { return "" }

func (shareAccessPage) GetFilter() string  { return "" }
func (shareAccessPage) GetOrderBy() string { return "" }

func (shareChildrenPage) GetFilter() string { return "" }

// CreateShare publishes a node.
func (s *ShareService) CreateShare(ctx context.Context, req *v1.CreateShareRequest) (*v1.Share, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	passwordHash := ""
	if req.GetPassword() != "" {
		plain, err := s.auth.DecodePassword(req.GetPassword())
		if err != nil {
			return nil, err
		}
		passwordHash, err = s.uc.HashPassword(plain)
		if err != nil {
			return nil, err
		}
	}
	in := biz.CreateShareInput{
		NodeID:       nodeID,
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		PasswordHash: passwordHash,
		PasswordHint: req.GetPasswordHint(),
		MaxDownloads: req.GetMaxDownloads(),
		Token:        req.GetToken(),
	}
	if perms, ok := parsePermissions(req.GetPermissions(), req.GetPermissionsMask()); ok {
		in.Permissions = perms
	}
	if req.GetExpiresAt() != nil {
		expires := req.GetExpiresAt().AsTime()
		in.ExpiresAt = &expires
	}
	created, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	return convertShare(created, s.uc.URL(created.Token), s.editableBy(ctx, created), nil), nil
}

// ListShares returns the share links the caller may see.
func (s *ShareService) ListShares(ctx context.Context, req *v1.ListSharesRequest) (*v1.ShareSet, error) {
	decl, err := shareDeclarations()
	if err != nil {
		return nil, err
	}
	opts, size, token, err := parseList(req, decl, sharePageSize, shareOrderPaths...)
	if err != nil {
		return nil, err
	}
	shares, total, err := s.uc.List(ctx, req.GetFilter(), opts...)
	if err != nil {
		return nil, err
	}
	set := &v1.ShareSet{
		Shares:    s.renderShares(ctx, shares, nil),
		TotalSize: total,
	}
	set.NextPageToken = nextPageToken(req, token, len(shares), int(size))
	return set, nil
}

// ListSharesByNode returns the share links attached to one node.
func (s *ShareService) ListSharesByNode(ctx context.Context, req *v1.ListSharesByNodeRequest) (*v1.ShareSet, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	decl, err := shareDeclarations()
	if err != nil {
		return nil, err
	}
	page := shareByNodePage{ListSharesByNodeRequest: req}
	opts, size, token, err := parseList(page, decl, sharePageSize, shareOrderPaths...)
	if err != nil {
		return nil, err
	}
	shares, err := s.uc.ListByNode(ctx, nodeID, opts...)
	if err != nil {
		return nil, err
	}
	node, access, err := s.nodes.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	set := &v1.ShareSet{
		Shares:    s.renderShares(ctx, shares, convertNodeOwned(node, access, "", "", 0)),
		TotalSize: int64(len(shares)),
	}
	set.NextPageToken = nextPageToken(page, token, len(shares), int(size))
	return set, nil
}

// AccessShare resolves a share token into the shared node and its first level
// of children. This endpoint is public.
func (s *ShareService) AccessShare(ctx context.Context, req *v1.AccessShareRequest) (*v1.ShareAccess, error) {
	share, err := s.resolve(ctx, req.GetToken())
	if err != nil {
		return nil, err
	}
	unlocked, fresh, err := s.unlockShare(share, req.GetAccessToken(), req.GetPassword())
	if err != nil {
		return nil, err
	}
	if !unlocked {
		return nil, biz.NodeLockedError(share.PasswordHint)
	}
	nodeID, err := parseOptionalUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	node, err := s.uc.ResolveNode(ctx, share, nodeID)
	if err != nil {
		return nil, err
	}
	page := shareAccessPage{AccessShareRequest: req}
	opts, size, token, err := parseList(page, nil, sharePageSize)
	if err != nil {
		return nil, err
	}
	children, _, err := s.uc.Children(ctx, share, node, opts...)
	if err != nil {
		return nil, err
	}
	if err := s.uc.MarkViewed(ctx, share.ID); err != nil {
		return nil, err
	}
	grant := s.publicGrant(share, unlocked)
	reply := &v1.ShareAccess{
		Share:         convertShare(share, s.uc.URL(share.Token), s.editableBy(ctx, share), nil),
		Node:          convertNodeOwned(node, grant, "", "", 0),
		Children:      make([]*v1.Node, 0, len(children)),
		NextPageToken: nextPageToken(page, token, len(children), int(size)),
	}
	for _, child := range children {
		reply.Children = append(reply.Children, convertNodeOwned(child, grant, "", "", 0))
	}
	// A protected link only needs a session token, and only on the call that
	// established access: an open link is usable by anyone holding the URL.
	if share.PasswordHash != "" && fresh {
		ttl := s.uc.AccessTTL()
		accessToken, err := s.auth.IssueShareToken(share.ID, ttl)
		if err != nil {
			return nil, err
		}
		reply.AccessToken = accessToken
		reply.ExpiresIn = int32(ttl.Seconds())
		reply.ExpiresAt = timestamppb.New(time.Now().Add(ttl))
	}
	return reply, nil
}

// ListShareChildren browses a shared folder. This endpoint is public and needs
// the access token for a password protected share.
func (s *ShareService) ListShareChildren(ctx context.Context, req *v1.ListShareChildrenRequest) (*v1.NodeSet, error) {
	share, err := s.resolve(ctx, req.GetToken())
	if err != nil {
		return nil, err
	}
	// The message carries no password field, so a protected share can only be
	// opened with a token issued by a previous AccessShare call.
	unlocked, _, err := s.unlockShare(share, req.GetAccessToken(), "")
	if err != nil {
		return nil, err
	}
	if !unlocked {
		return nil, biz.NodeLockedError(share.PasswordHint)
	}
	nodeID, err := parseOptionalUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	node, err := s.uc.ResolveNode(ctx, share, nodeID)
	if err != nil {
		return nil, err
	}
	page := shareChildrenPage{ListShareChildrenRequest: req}
	opts, size, token, err := parseList(page, nil, sharePageSize)
	if err != nil {
		return nil, err
	}
	children, _, err := s.uc.Children(ctx, share, node, opts...)
	if err != nil {
		return nil, err
	}
	grant := s.publicGrant(share, unlocked)
	set := &v1.NodeSet{Nodes: make([]*v1.Node, 0, len(children))}
	for _, child := range children {
		set.Nodes = append(set.Nodes, convertNodeOwned(child, grant, "", "", 0))
	}
	set.NextPageToken = nextPageToken(page, token, len(children), int(size))
	return set, nil
}

// GetShareDownloadUrl returns a signed URL for a node inside a share. This
// endpoint is public and honours the share capability set and download budget.
func (s *ShareService) GetShareDownloadUrl(ctx context.Context, req *v1.GetShareDownloadUrlRequest) (*v1.SignedUrl, error) {
	share, err := s.resolve(ctx, req.GetToken())
	if err != nil {
		return nil, err
	}
	unlocked, _, err := s.unlockShare(share, req.GetAccessToken(), "")
	if err != nil {
		return nil, err
	}
	if !unlocked {
		return nil, biz.NodeLockedError(share.PasswordHint)
	}
	nodeID, err := parseOptionalUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	node, err := s.uc.ResolveNode(ctx, share, nodeID)
	if err != nil {
		return nil, err
	}
	signed, err := s.uc.Download(ctx, share, node, unlocked, req.GetExpiresInSeconds(), req.GetInline(), "")
	if err != nil {
		return nil, err
	}
	if err := s.uc.MarkDownloaded(ctx, share.ID); err != nil {
		return nil, err
	}
	return convertSignedURL(signed), nil
}

// GetShare returns one share link.
func (s *ShareService) GetShare(ctx context.Context, req *v1.GetShareRequest) (*v1.Share, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	share, err := s.uc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return convertShare(share, s.uc.URL(share.Token), s.editableBy(ctx, share), nil), nil
}

// UpdateShare changes the capability set, password, expiry or budget of an
// existing share link.
func (s *ShareService) UpdateShare(ctx context.Context, req *v1.UpdateShareRequest) (*v1.Share, error) {
	if len(req.GetUpdateMask().GetPaths()) == 0 {
		return nil, biz.ErrInvalidArgument
	}
	id, err := parseUUID(req.GetShare().GetId())
	if err != nil {
		return nil, err
	}
	in := biz.UpdateShareInput{ID: id}
	for _, path := range req.GetUpdateMask().GetPaths() {
		switch strings.TrimSpace(path) {
		case "*", "name":
			name := req.GetShare().GetName()
			in.Name = &name
		case "description":
			description := req.GetShare().GetDescription()
			in.Description = &description
		case "permissions", "permissions_mask":
			perms, ok := parsePermissions(req.GetShare().GetPermissions(), req.GetShare().GetPermissionsMask())
			if !ok {
				return nil, biz.ErrInvalidArgument
			}
			in.Permissions = &perms
		case "password":
			if req.GetRemovePassword() {
				in.ClearPassword = true
				break
			}
			if req.GetPassword() == "" {
				return nil, biz.ErrInvalidArgument
			}
			plain, err := s.auth.DecodePassword(req.GetPassword())
			if err != nil {
				return nil, err
			}
			hash, err := s.uc.HashPassword(plain)
			if err != nil {
				return nil, err
			}
			in.PasswordHash = &hash
			hint := req.GetShare().GetPasswordHint()
			in.PasswordHint = &hint
		case "password_hint":
			hint := req.GetShare().GetPasswordHint()
			in.PasswordHint = &hint
		case "expires_at":
			if req.GetShare().GetExpiresAt() == nil {
				in.ClearExpiry = true
				break
			}
			expires := req.GetShare().GetExpiresAt().AsTime()
			in.ExpiresAt = &expires
		case "max_downloads":
			maxDownloads := req.GetShare().GetMaxDownloads()
			in.MaxDownloads = &maxDownloads
		case "status":
			status := parseShareStatus(req.GetShare().GetStatus())
			in.Status = &status
		default:
			return nil, biz.ErrInvalidArgument
		}
	}
	updated, err := s.uc.Update(ctx, in)
	if err != nil {
		return nil, err
	}
	return convertShare(updated, s.uc.URL(updated.Token), s.editableBy(ctx, updated), nil), nil
}

// DeleteShare revokes a share link immediately.
func (s *ShareService) DeleteShare(ctx context.Context, req *v1.DeleteShareRequest) (*emptypb.Empty, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.uc.Delete(ctx, id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// resolve loads a share for an anonymous endpoint. A blank or unknown token is
// reported as not found, and the returned share is never nil.
func (s *ShareService) resolve(ctx context.Context, token string) (*biz.Share, error) {
	share, err := s.uc.Resolve(ctx, token)
	if err != nil {
		if errors.Is(err, biz.ErrInvalidArgument) {
			return nil, biz.ErrNotFound
		}
		return nil, err
	}
	if share == nil {
		return nil, biz.ErrNotFound
	}
	return share, nil
}

// unlockShare reports whether a visitor may open a share. A link without a
// password is open; a protected one accepts the access token of a previous
// AccessShare call, or the password itself. fresh reports that this call
// established access with the password.
func (s *ShareService) unlockShare(share *biz.Share, accessToken, password string) (unlocked, fresh bool, err error) {
	if share == nil {
		return false, false, biz.ErrNotFound
	}
	if share.PasswordHash == "" {
		return true, false, nil
	}
	if accessToken != "" {
		if id, err := s.auth.ParseShareToken(accessToken); err == nil && id == share.ID {
			return true, false, nil
		}
	}
	if password == "" {
		return false, false, nil
	}
	plain, err := s.auth.DecodePassword(password)
	if err != nil {
		return false, false, err
	}
	if err := s.uc.VerifyPassword(share, plain); err != nil {
		return false, false, err
	}
	return true, true, nil
}

// publicGrant is the access a visitor holds on the nodes of a share. A share
// conveys its own capability set and is public by definition.
func (s *ShareService) publicGrant(share *biz.Share, unlocked bool) biz.NodeAccess {
	return biz.NodeAccess{Perms: s.uc.Grant(share, unlocked).Perms, Visibility: biz.VisibilityPublic}
}

// editableBy reports whether the caller may change a share link.
func (s *ShareService) editableBy(ctx context.Context, share *biz.Share) bool {
	caller := biz.CallerFromContext(ctx)
	return share.OwnerID == caller.UserID || caller.Has(biz.PermUserManage)
}

// renderShares renders a page of share links. node is attached to every row
// when the page lists the shares of a single node.
func (s *ShareService) renderShares(ctx context.Context, shares []*biz.Share, node *v1.Node) []*v1.Share {
	out := make([]*v1.Share, 0, len(shares))
	for _, share := range shares {
		out = append(out, convertShare(share, s.uc.URL(share.Token), s.editableBy(ctx, share), node))
	}
	return out
}
