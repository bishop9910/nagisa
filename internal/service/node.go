package service

import (
	"context"
	"strings"
	"time"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"

	"github.com/google/uuid"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/filtering"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// NodeService implements the file tree API on top of the node and file
// usecases.
type NodeService struct {
	v1.UnimplementedNodeServiceServer

	uc    *biz.NodeUsecase
	files *biz.FileUsecase
	auth  *biz.AuthUsecase
	users *biz.UserUsecase
	share *biz.ShareUsecase
}

// NewNodeService new a node service.
func NewNodeService(uc *biz.NodeUsecase, files *biz.FileUsecase, auth *biz.AuthUsecase, users *biz.UserUsecase, share *biz.ShareUsecase) *NodeService {
	return &NodeService{uc: uc, files: files, auth: auth, users: users, share: share}
}

// nodeDeclarations describes the filterable fields of a node.
func nodeDeclarations() (*filtering.Declarations, error) {
	return declarations(
		filtering.DeclareIdent("id", filtering.TypeString),
		filtering.DeclareIdent("name", filtering.TypeString),
		filtering.DeclareIdent("kind", filtering.TypeInt),
		filtering.DeclareIdent("mime_type", filtering.TypeString),
		filtering.DeclareIdent("extension", filtering.TypeString),
		filtering.DeclareIdent("size", filtering.TypeInt),
		filtering.DeclareIdent("owner_id", filtering.TypeString),
		filtering.DeclareIdent("status", filtering.TypeInt),
		filtering.DeclareIdent("path", filtering.TypeString),
		filtering.DeclareIdent("trashed_at", filtering.TypeTimestamp),
		filtering.DeclareIdent("created_at", filtering.TypeTimestamp),
		filtering.DeclareIdent("updated_at", filtering.TypeTimestamp),
	)
}

// nodeOrderPaths lists the orderable fields of a node.
var nodeOrderPaths = []string{"name", "size", "kind", "mime_type", "extension", "status", "created_at", "updated_at"}

// pathResolver memoises the human readable path of parent folders so a listing
// costs one ancestor walk per distinct parent instead of one per row.
type pathResolver struct {
	uc    *biz.NodeUsecase
	cache map[uuid.UUID]string
}

func newPathResolver(uc *biz.NodeUsecase) *pathResolver {
	return &pathResolver{uc: uc, cache: map[uuid.UUID]string{}}
}

func (r *pathResolver) of(ctx context.Context, n *biz.Node) string {
	if n == nil {
		return ""
	}
	base, ok := r.cache[n.ParentID]
	if !ok {
		base = r.base(ctx, n.ParentID)
		r.cache[n.ParentID] = base
	}
	return strings.TrimRight(base, "/") + "/" + n.Name
}

func (r *pathResolver) base(ctx context.Context, parentID uuid.UUID) string {
	if parentID == uuid.Nil {
		return "/" + r.uc.RootName()
	}
	parent, _, err := r.uc.GetNode(ctx, parentID)
	if err != nil {
		return "/" + r.uc.RootName()
	}
	return r.of(ctx, parent)
}

// renderNodes converts a page of outcomes into replies, resolving the display
// path, the owner name and the share count in batches.
func (s *NodeService) renderNodes(ctx context.Context, outcomes []biz.Outcome, withPath bool) []*v1.Node {
	if len(outcomes) == 0 {
		return nil
	}
	ownerIDs := make([]uuid.UUID, 0, len(outcomes))
	nodeIDs := make([]uuid.UUID, 0, len(outcomes))
	for _, o := range outcomes {
		ownerIDs = append(ownerIDs, o.Node.OwnerID)
		nodeIDs = append(nodeIDs, o.Node.ID)
	}
	owners, err := s.users.Names(ctx, ownerIDs)
	if err != nil {
		owners = map[uuid.UUID]string{}
	}
	counts := map[uuid.UUID]int64{}
	if s.share != nil {
		if c, err := s.share.CountsByNodes(ctx, nodeIDs); err == nil {
			counts = c
		}
	}
	var resolver *pathResolver
	if withPath {
		resolver = newPathResolver(s.uc)
	}
	out := make([]*v1.Node, 0, len(outcomes))
	for _, o := range outcomes {
		display := ""
		if withPath {
			display = resolver.of(ctx, o.Node)
		}
		out = append(out, convertNodeOwned(o.Node, o.Access, display, owners[o.Node.OwnerID], int32(counts[o.Node.ID])))
	}
	return out
}

// ListNodes returns the children of a folder.
func (s *NodeService) ListNodes(ctx context.Context, req *v1.ListNodesRequest) (*v1.NodeSet, error) {
	parentID, err := parseOptionalUUID(req.GetParentId())
	if err != nil {
		return nil, err
	}
	decl, err := nodeDeclarations()
	if err != nil {
		return nil, err
	}
	opts, size, token, err := parseList(req, decl, 1000, nodeOrderPaths...)
	if err != nil {
		return nil, err
	}
	in := biz.ListNodesInput{
		ParentID:       parentID,
		Recursive:      req.GetRecursive(),
		MaxDepth:       req.GetMaxDepth(),
		IncludeTrashed: req.GetIncludeTrashed(),
	}
	outcomes, err := s.uc.ListNodes(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	total, err := s.uc.CountNodes(ctx, in)
	if err != nil {
		total = int64(len(outcomes))
	}
	set := &v1.NodeSet{
		Nodes:     s.renderNodes(ctx, outcomes, true),
		TotalSize: total,
	}
	for _, o := range outcomes {
		if o.Node.IsFile() {
			set.TotalBytes += o.Node.Size
		}
	}
	set.NextPageToken = nextPageToken(req, token, len(outcomes), int(size))
	return set, nil
}

// SearchNodes searches the tree visible to the caller.
func (s *NodeService) SearchNodes(ctx context.Context, req *v1.SearchNodesRequest) (*v1.NodeSet, error) {
	scope, err := parseOptionalUUID(req.GetScopeNodeId())
	if err != nil {
		return nil, err
	}
	owner, err := parseOptionalUUID(req.GetOwnerId())
	if err != nil {
		return nil, err
	}
	opts, size, token, err := parseOrderedList(req, 1000, "name", "size", "updated_at", "created_at")
	if err != nil {
		return nil, err
	}
	in := biz.SearchNodesInput{
		Query:            req.GetQuery(),
		ScopeNodeID:      scope,
		Kind:             parseNodeKind(req.GetKind()),
		MimeType:         req.GetMimeType(),
		OwnerID:          owner,
		MinSize:          req.GetMinSize(),
		MaxSize:          req.GetMaxSize(),
		MatchDescription: req.GetMatchDescription(),
	}
	if req.GetUpdatedAfter() != nil {
		t := req.GetUpdatedAfter().AsTime()
		in.UpdatedAfter = &t
	}
	if req.GetUpdatedBefore() != nil {
		t := req.GetUpdatedBefore().AsTime()
		in.UpdatedBefore = &t
	}
	outcomes, total, err := s.uc.SearchNodes(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	set := &v1.NodeSet{
		Nodes:         s.renderNodes(ctx, outcomes, true),
		TotalSize:     total,
		NextPageToken: nextPageToken(req, token, len(outcomes), int(size)),
	}
	return set, nil
}

// GetNodeTree returns a folder subtree expanded to the requested depth.
func (s *NodeService) GetNodeTree(ctx context.Context, req *v1.GetNodeTreeRequest) (*v1.NodeTree, error) {
	rootID, err := parseOptionalUUID(req.GetRootId())
	if err != nil {
		return nil, err
	}
	depth := req.GetDepth()
	if depth <= 0 {
		depth = 1
	}
	// page_size bounds the children of each folder rather than the total,
	// which keeps one wide folder from drowning the whole reply.
	perFolder := int(req.GetPageSize())
	if perFolder <= 0 {
		perFolder = defaultTreePageSize
	}
	if perFolder > maxTreePageSize {
		perFolder = maxTreePageSize
	}
	outcomes, err := s.uc.Tree(ctx, rootID, depth)
	if err != nil {
		return nil, err
	}
	resolver := newPathResolver(s.uc)
	rendered := map[uuid.UUID]*v1.NodeTree{}
	var root *v1.NodeTree
	if rootID != uuid.Nil {
		node, access, err := s.uc.GetNode(ctx, rootID)
		if err != nil {
			return nil, err
		}
		owners, _ := s.users.Names(ctx, []uuid.UUID{node.OwnerID})
		root = &v1.NodeTree{Node: convertNodeOwned(node, access, resolver.of(ctx, node), owners[node.OwnerID], 0)}
	}
	for _, o := range outcomes {
		owners, _ := s.users.Names(ctx, []uuid.UUID{o.Node.OwnerID})
		rendered[o.Node.ID] = &v1.NodeTree{
			Node: convertNodeOwned(o.Node, o.Access, resolver.of(ctx, o.Node), owners[o.Node.OwnerID], 0),
		}
	}
	appendChild := func(parent *v1.NodeTree, child *v1.NodeTree) {
		if parent == nil || child == nil || len(parent.Children) >= perFolder {
			return
		}
		parent.Children = append(parent.Children, child)
	}
	for _, o := range outcomes {
		tree := rendered[o.Node.ID]
		if parent, ok := rendered[o.Node.ParentID]; ok {
			appendChild(parent, tree)
		} else if root != nil && o.Node.ParentID == rootID {
			appendChild(root, tree)
		}
	}
	if root == nil {
		root = &v1.NodeTree{Node: &v1.Node{
			Id:          "",
			Name:        s.uc.RootName(),
			Kind:        v1.NodeKind_NODE_KIND_FOLDER,
			Visibility:  v1.Visibility_VISIBILITY_PRIVATE,
			Description: "虚拟根目录",
		}}
		for _, o := range outcomes {
			if o.Node.ParentID == uuid.Nil {
				appendChild(root, rendered[o.Node.ID])
			}
		}
	}
	return root, nil
}

// ListTrash returns the trashed nodes visible to the caller: the top entry of
// every trashed subtree, or the trashed children of one folder.
func (s *NodeService) ListTrash(ctx context.Context, req *v1.ListTrashRequest) (*v1.NodeSet, error) {
	parentID, err := parseOptionalUUID(req.GetOriginalParentId())
	if err != nil {
		return nil, err
	}
	decl, err := nodeDeclarations()
	if err != nil {
		return nil, err
	}
	opts, size, token, err := parseList(req, decl, 1000, "name", "size", "trashed_at", "updated_at")
	if err != nil {
		return nil, err
	}
	outcomes, total, err := s.uc.TrashNodes(ctx, parentID, opts...)
	if err != nil {
		return nil, err
	}
	set := &v1.NodeSet{
		Nodes:         s.renderNodes(ctx, outcomes, false),
		TotalSize:     total,
		NextPageToken: nextPageToken(req, token, len(outcomes), int(size)),
	}
	return set, nil
}

// GetNode returns a single node.
func (s *NodeService) GetNode(ctx context.Context, req *v1.GetNodeRequest) (*v1.Node, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	node, access, err := s.uc.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	resolver := newPathResolver(s.uc)
	owners, _ := s.users.Names(ctx, []uuid.UUID{node.OwnerID})
	counts := map[uuid.UUID]int64{}
	if s.share != nil {
		counts, _ = s.share.CountsByNodes(ctx, []uuid.UUID{node.ID})
	}
	return convertNodeOwned(node, access, resolver.of(ctx, node), owners[node.OwnerID], int32(counts[node.ID])), nil
}

// GetNodePath returns the breadcrumb of a node.
func (s *NodeService) GetNodePath(ctx context.Context, req *v1.GetNodePathRequest) (*v1.NodePath, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	ancestors, node, err := s.uc.Path(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &v1.NodePath{
		Ancestors:   make([]*v1.Node, 0, len(ancestors)),
		DisplayPath: biz.DisplayPathOf(ancestors, node, s.uc.RootName()),
	}
	for _, a := range ancestors {
		access, err := s.uc.Evaluate(ctx, biz.CallerFromContext(ctx), a)
		if err != nil {
			return nil, err
		}
		out.Ancestors = append(out.Ancestors, convertNodeOwned(a, access, "", "", 0))
	}
	access, err := s.uc.Evaluate(ctx, biz.CallerFromContext(ctx), node)
	if err != nil {
		return nil, err
	}
	out.Node = convertNodeOwned(node, access, out.DisplayPath, "", 0)
	return out, nil
}

// GetNodeStats returns aggregate counters for a subtree.
func (s *NodeService) GetNodeStats(ctx context.Context, req *v1.GetNodeStatsRequest) (*v1.NodeStats, error) {
	id, err := parseOptionalUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	stats, err := s.uc.Stats(ctx, id)
	if err != nil {
		return nil, err
	}
	return &v1.NodeStats{
		NodeId:          uuidOrEmpty(stats.NodeID),
		FileCount:       stats.FileCount,
		FolderCount:     stats.FolderCount,
		TotalSize:       stats.TotalSize,
		TrashedCount:    stats.TrashedCount,
		TrashedSize:     stats.TrashedSize,
		LargestFileSize: stats.LargestFileSize,
		LargestFileName: stats.LargestFileName,
		SizeByCategory:  stats.CategorySizes,
	}, nil
}

// GetNodeAcl returns the ACL of a node.
func (s *NodeService) GetNodeAcl(ctx context.Context, req *v1.GetNodeAclRequest) (*v1.NodeAcl, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	entries, inherited, access, err := s.uc.Acl(ctx, id)
	if err != nil {
		return nil, err
	}
	names := s.aclSubjectNames(ctx, entries, inherited)
	permissionValues, permissionMask := convertPermissions(access.Perms)
	return &v1.NodeAcl{
		NodeId:                   id.String(),
		Entries:                  s.renderAcl(entries, names),
		InheritedEntries:         s.renderAcl(inherited, names),
		EffectivePermissions:     permissionValues,
		EffectivePermissionsMask: permissionMask,
		Owned:                    access.Owned,
		Editable:                 access.Perms.Has(biz.PermAclManage),
	}, nil
}

// aclSubjectNames resolves the display names of the subjects of a set of
// entries in one lookup.
func (s *NodeService) aclSubjectNames(ctx context.Context, groups ...[]*biz.AclEntry) map[string]string {
	ids := make([]uuid.UUID, 0, 8)
	for _, group := range groups {
		for _, e := range group {
			if e.SubjectType != biz.SubjectTypeUser {
				continue
			}
			if id, err := uuid.Parse(e.SubjectID); err == nil {
				ids = append(ids, id)
			}
		}
	}
	names := map[string]string{}
	if len(ids) == 0 {
		return names
	}
	resolved, err := s.users.Names(ctx, ids)
	if err != nil {
		return names
	}
	for _, e := range flattenAcl(groups...) {
		if e.SubjectType == biz.SubjectTypeUser {
			if id, err := uuid.Parse(e.SubjectID); err == nil {
				names[e.SubjectID] = resolved[id]
			}
		} else if e.SubjectType == biz.SubjectTypeRole {
			names[e.SubjectID] = e.SubjectID
		} else {
			names[e.SubjectID] = "所有人"
		}
	}
	return names
}

func flattenAcl(groups ...[]*biz.AclEntry) []*biz.AclEntry {
	out := make([]*biz.AclEntry, 0, 8)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func (s *NodeService) renderAcl(entries []*biz.AclEntry, names map[string]string) []*v1.AclEntry {
	out := make([]*v1.AclEntry, 0, len(entries))
	for _, e := range entries {
		name := names[e.SubjectID]
		if e.SubjectType == biz.SubjectTypeEveryone && name == "" {
			name = "所有人"
		}
		out = append(out, convertAclEntry(e, name))
	}
	return out
}

// CreateFolder creates a folder with its description, visibility, password and
// ACL in one transaction.
func (s *NodeService) CreateFolder(ctx context.Context, req *v1.CreateFolderRequest) (*v1.Node, error) {
	folder := req.GetFolder()
	parentID, err := parseOptionalUUID(folder.GetParentId())
	if err != nil {
		return nil, err
	}
	passwordHash := ""
	if req.GetPassword() != "" {
		plain, err := s.auth.DecodePassword(req.GetPassword())
		if err != nil {
			return nil, err
		}
		passwordHash, err = s.auth.HashPassword(plain)
		if err != nil {
			return nil, err
		}
	}
	in := biz.CreateFolderInput{
		ParentID:     parentID,
		Name:         folder.GetName(),
		Description:  folder.GetDescription(),
		Visibility:   parseVisibility(folder.GetVisibility()),
		PasswordHash: passwordHash,
		PasswordHint: folder.GetPasswordHint(),
		Metadata:     folder.GetMetadata(),
		Policy:       parseConflictPolicy(req.GetConflictPolicy()),
		Acl:          convertAclEntries(req.GetAcl()),
	}
	created, access, err := s.uc.CreateFolder(ctx, in)
	if err != nil {
		return nil, err
	}
	resolver := newPathResolver(s.uc)
	owners, _ := s.users.Names(ctx, []uuid.UUID{created.OwnerID})
	return convertNodeOwned(created, access, resolver.of(ctx, created), owners[created.OwnerID], 0), nil
}

// convertAclEntries parses incoming entries into domain entries.
func convertAclEntries(in []*v1.AclEntry) []*biz.AclEntry {
	out := make([]*biz.AclEntry, 0, len(in))
	for _, e := range in {
		perms, _ := parsePermissions(e.GetPermissions(), e.GetPermissionsMask())
		out = append(out, &biz.AclEntry{
			SubjectType: parseSubjectType(e.GetSubjectType()),
			SubjectID:   e.GetSubjectId(),
			Effect:      parseEffect(e.GetEffect()),
			Permissions: perms,
			Inherit:     e.GetInherit(),
		})
	}
	return out
}

// UpdateNode applies a partial update.
func (s *NodeService) UpdateNode(ctx context.Context, req *v1.UpdateNodeRequest) (*v1.Node, error) {
	if req.GetNode().GetId() == "" || req.GetUpdateMask() == nil || len(req.GetUpdateMask().GetPaths()) == 0 {
		return nil, biz.ErrInvalidArgument
	}
	id, err := parseUUID(req.GetNode().GetId())
	if err != nil {
		return nil, err
	}
	current, _, err := s.uc.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	in := biz.UpdateNodeInput{ID: id, Policy: parseConflictPolicy(req.GetConflictPolicy())}
	for _, path := range req.GetUpdateMask().GetPaths() {
		switch strings.TrimSpace(path) {
		case "*", "name":
			name := req.GetNode().GetName()
			in.Name = &name
		case "description":
			description := req.GetNode().GetDescription()
			in.Description = &description
		case "visibility":
			visibility := parseVisibility(req.GetNode().GetVisibility())
			in.Visibility = &visibility
		case "metadata":
			in.Metadata = req.GetNode().GetMetadata()
		case "parent_id":
			parentID, err := parseOptionalUUID(req.GetNode().GetParentId())
			if err != nil {
				return nil, err
			}
			in.ParentID = &parentID
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
			hash, err := s.auth.HashPassword(plain)
			if err != nil {
				return nil, err
			}
			in.PasswordHash = &hash
			hint := req.GetNode().GetPasswordHint()
			in.PasswordHint = &hint
		default:
			return nil, biz.ErrInvalidArgument
		}
	}
	updated, access, err := s.uc.UpdateNode(ctx, in)
	if err != nil {
		return nil, err
	}
	resolver := newPathResolver(s.uc)
	_ = current
	return convertNodeOwned(updated, access, resolver.of(ctx, updated), "", 0), nil
}

// SetNodeAcl replaces the ACL of a node.
func (s *NodeService) SetNodeAcl(ctx context.Context, req *v1.SetNodeAclRequest) (*v1.NodeAcl, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	entries, access, err := s.uc.SetNodeAcl(ctx, biz.SetNodeAclInput{
		NodeID:    nodeID,
		Entries:   convertAclEntries(req.GetEntries()),
		Recursive: req.GetRecursive(),
	})
	if err != nil {
		return nil, err
	}
	names := s.aclSubjectNames(ctx, entries)
	permissionValues, permissionMask := convertPermissions(access.Perms)
	return &v1.NodeAcl{
		NodeId:                   nodeID.String(),
		Entries:                  s.renderAcl(entries, names),
		EffectivePermissions:     permissionValues,
		EffectivePermissionsMask: permissionMask,
		Owned:                    access.Owned,
		Editable:                 access.Perms.Has(biz.PermAclManage),
	}, nil
}

// UnlockNode exchanges a folder password for a short lived node token.
func (s *NodeService) UnlockNode(ctx context.Context, req *v1.UnlockNodeRequest) (*v1.NodeUnlock, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	plain, err := s.auth.DecodePassword(req.GetPassword())
	if err != nil {
		return nil, err
	}
	node, err := s.uc.Unlock(ctx, nodeID, plain)
	if err != nil {
		return nil, err
	}
	token, expires, err := s.auth.IssueNodeToken([]uuid.UUID{node.ID})
	if err != nil {
		return nil, err
	}
	return &v1.NodeUnlock{
		NodeId:      node.ID.String(),
		UnlockToken: token,
		ExpiresIn:   int32(timeUntil(expires).Seconds()),
		ExpiresAt:   timestamppb.New(expires),
	}, nil
}

// MoveNodes moves a batch of nodes atomically.
func (s *NodeService) MoveNodes(ctx context.Context, req *v1.MoveNodesRequest) (*v1.NodeSet, error) {
	ids, err := parseUUIDs(req.GetIds())
	if err != nil {
		return nil, err
	}
	target, err := parseOptionalUUID(req.GetTargetParentId())
	if err != nil {
		return nil, err
	}
	outcomes, err := s.uc.MoveNodes(ctx, biz.MoveNodesInput{
		IDs:            ids,
		TargetParentID: target,
		Policy:         parseConflictPolicy(req.GetConflictPolicy()),
		NewNames:       req.GetNewNames(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.NodeSet{Nodes: s.renderNodes(ctx, outcomes, true)}, nil
}

// CopyNodes copies nodes into another folder.
func (s *NodeService) CopyNodes(ctx context.Context, req *v1.CopyNodesRequest) (*v1.NodeSet, error) {
	ids, err := parseUUIDs(req.GetIds())
	if err != nil {
		return nil, err
	}
	target, err := parseOptionalUUID(req.GetTargetParentId())
	if err != nil {
		return nil, err
	}
	outcomes, err := s.files.CopyNodes(ctx, biz.CopyNodesInput{
		IDs:            ids,
		TargetParentID: target,
		Policy:         parseConflictPolicy(req.GetConflictPolicy()),
		NewNames:       req.GetNewNames(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.NodeSet{Nodes: s.renderNodes(ctx, outcomes, true)}, nil
}

// DeleteNodes moves nodes to the trash, or purges them.
func (s *NodeService) DeleteNodes(ctx context.Context, req *v1.DeleteNodesRequest) (*v1.DeleteNodesReply, error) {
	ids, err := parseUUIDs(req.GetIds())
	if err != nil {
		return nil, err
	}
	deleted, affected, err := s.uc.DeleteNodes(ctx, biz.DeleteNodesInput{IDs: ids, Permanent: req.GetPermanent()})
	if err != nil {
		return nil, err
	}
	reply := &v1.DeleteNodesReply{AffectedCount: affected, ReclaimedBytes: 0}
	for _, id := range deleted {
		reply.DeletedIds = append(reply.DeletedIds, id.String())
	}
	return reply, nil
}

// RestoreNodes restores nodes from the trash.
func (s *NodeService) RestoreNodes(ctx context.Context, req *v1.RestoreNodesRequest) (*v1.NodeSet, error) {
	ids, err := parseUUIDs(req.GetIds())
	if err != nil {
		return nil, err
	}
	target, err := parseOptionalUUID(req.GetTargetParentId())
	if err != nil {
		return nil, err
	}
	outcomes, err := s.uc.RestoreNodes(ctx, biz.RestoreNodesInput{
		IDs:            ids,
		TargetParentID: target,
		Policy:         parseConflictPolicy(req.GetConflictPolicy()),
	})
	if err != nil {
		return nil, err
	}
	return &v1.NodeSet{Nodes: s.renderNodes(ctx, outcomes, true)}, nil
}

// PurgeNodes permanently deletes nodes.
func (s *NodeService) PurgeNodes(ctx context.Context, req *v1.PurgeNodesRequest) (*v1.PurgeNodesReply, error) {
	ids, err := parseUUIDs(req.GetIds())
	if err != nil {
		return nil, err
	}
	purged, reclaimed, err := s.uc.PurgeNodes(ctx, ids)
	if err != nil {
		return nil, err
	}
	reply := &v1.PurgeNodesReply{ReclaimedBytes: reclaimed, AffectedCount: int64(len(purged))}
	for _, id := range purged {
		reply.PurgedIds = append(reply.PurgedIds, id.String())
	}
	return reply, nil
}

// EmptyTrash purges every trashed node visible to the caller.
func (s *NodeService) EmptyTrash(ctx context.Context, req *v1.EmptyTrashRequest) (*v1.PurgeNodesReply, error) {
	var before *time.Time
	if req.GetTrashedBefore() != nil {
		t := req.GetTrashedBefore().AsTime()
		before = &t
	}
	purged, reclaimed, err := s.uc.EmptyTrash(ctx, before)
	if err != nil {
		return nil, err
	}
	reply := &v1.PurgeNodesReply{ReclaimedBytes: reclaimed, AffectedCount: int64(len(purged))}
	for _, id := range purged {
		reply.PurgedIds = append(reply.PurgedIds, id.String())
	}
	return reply, nil
}

// ListNodeVersions returns the content history of a file node.
func (s *NodeService) ListNodeVersions(ctx context.Context, req *v1.ListNodeVersionsRequest) (*v1.NodeVersionSet, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	opts, size, token, err := parsePagedList(req, 200)
	if err != nil {
		return nil, err
	}
	versions, _, err := s.files.Versions(ctx, nodeID, opts...)
	if err != nil {
		return nil, err
	}
	node, _, err := s.uc.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	set := &v1.NodeVersionSet{Versions: make([]*v1.NodeVersion, 0, len(versions))}
	for _, v := range versions {
		set.Versions = append(set.Versions, convertVersion(v, node.StorageKey))
	}
	set.NextPageToken = nextPageToken(req, token, len(versions), int(size))
	return set, nil
}

// RestoreNodeVersion promotes an older revision to be the current content.
func (s *NodeService) RestoreNodeVersion(ctx context.Context, req *v1.RestoreNodeVersionRequest) (*v1.Node, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	versionID, err := parseUUID(req.GetVersionId())
	if err != nil {
		return nil, err
	}
	node, err := s.files.RestoreVersion(ctx, nodeID, versionID)
	if err != nil {
		return nil, err
	}
	access, err := s.uc.Evaluate(ctx, biz.CallerFromContext(ctx), node)
	if err != nil {
		return nil, err
	}
	resolver := newPathResolver(s.uc)
	return convertNodeOwned(node, access, resolver.of(ctx, node), "", 0), nil
}

// DeleteNodeVersion drops one retained revision.
func (s *NodeService) DeleteNodeVersion(ctx context.Context, req *v1.DeleteNodeVersionRequest) (*emptypb.Empty, error) {
	nodeID, err := parseUUID(req.GetNodeId())
	if err != nil {
		return nil, err
	}
	versionID, err := parseUUID(req.GetVersionId())
	if err != nil {
		return nil, err
	}
	if err := s.files.DeleteVersion(ctx, nodeID, versionID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// The fieldmask helper is used by the update paths that rely on AIP semantics
// for the request masks they accept.
var _ = fieldmask.Update
