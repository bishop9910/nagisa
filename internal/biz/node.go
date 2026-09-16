package biz

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

// NewID returns a time-ordered application identifier. Identifiers are minted
// in the domain layer so an entity's materialised path can be computed before
// it is written.
func NewID() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}

// Node is a file or a folder. Folders are nodes of kind NodeKindFolder so the
// description, ACL, visibility and password model applies to both.
type Node struct {
	ID           uuid.UUID
	ParentID     uuid.UUID
	Name         string
	NameLower    string
	Kind         NodeKind
	OwnerID      uuid.UUID
	Size         int64
	MimeType     string
	Extension    string
	Etag         string
	StorageKey   string
	Status       NodeStatus
	Description  string
	Visibility   Visibility
	PasswordHash string
	PasswordHint string
	HasThumbnail bool
	Metadata     map[string]string

	// Path is the materialised chain of ancestor ids, for example
	// "/<id1>/<id2>/", which turns a subtree into a single prefix scan.
	Path  string
	Depth int32

	ChildCount  int64
	FileCount   int64
	FolderCount int64
	SubtreeSize int64

	CurrentVersionID *uuid.UUID
	VersionCount     int32

	TrashedAt        *time.Time
	OriginalParentID uuid.UUID

	CreatedBy uuid.UUID
	UpdatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsFolder reports whether the node is a folder.
func (n *Node) IsFolder() bool { return n != nil && n.Kind == NodeKindFolder }

// IsFile reports whether the node is a file.
func (n *Node) IsFile() bool { return n != nil && n.Kind == NodeKindFile }

// AclEntry grants or denies a permission set for one subject on one node.
type AclEntry struct {
	ID          uuid.UUID
	NodeID      uuid.UUID
	SubjectType SubjectType
	SubjectID   string
	Effect      Effect
	Permissions PermMask
	Inherit     bool
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
}

// NodeQuery describes a node listing. Zero fields mean "no constraint", except
// for Status, where nil selects active nodes unless IncludeTrashed is set.
type NodeQuery struct {
	// ParentID selects the direct children of a folder. The zero id selects
	// root level nodes.
	ParentID *uuid.UUID
	// PathPrefix selects a whole subtree, including the node itself.
	PathPrefix string
	// Roots selects root level nodes, restricted to OwnerID when it is set.
	Roots bool
	// OwnerID restricts the result to one account.
	OwnerID *uuid.UUID
	// Status restricts the lifecycle state.
	Status *NodeStatus
	// Kind restricts the node kind.
	Kind *NodeKind
	// Names restricts the result to these names, compared case-insensitively.
	Names []string
	// ExcludeIDs removes nodes from the result.
	ExcludeIDs []uuid.UUID
	// IncludeTrashed keeps trashed rows in an otherwise active listing.
	IncludeTrashed bool
	// MaxDepth, when non-zero, bounds the absolute depth of a PathPrefix
	// listing. The caller converts its relative limit into an absolute one.
	MaxDepth int32
}

// NodeRepo persists the file tree.
type NodeRepo interface {
	FindNodeByID(context.Context, uuid.UUID) (*Node, error)
	FindNodesByIDs(context.Context, []uuid.UUID) ([]*Node, error)
	ListNodes(context.Context, NodeQuery, ...ListOption) ([]*Node, error)
	CountNodes(context.Context, NodeQuery, ...ListOption) (int64, error)
	CreateNode(context.Context, *Node) (*Node, error)
	UpdateNode(context.Context, *Node) (*Node, error)
	// RelocateSubtree sets the parent, path and depth of a node and rewrites
	// the materialised path and depth of every descendant in one statement.
	RelocateSubtree(context.Context, *Node, uuid.UUID, string, int32) error
	// RenameNode changes the name of a single node.
	RenameNode(context.Context, uuid.UUID, string) error
	// SetSubtreeStatus flips the lifecycle state of a node and its subtree.
	SetSubtreeStatus(context.Context, *Node, NodeStatus, *time.Time) (int64, error)
	// DeleteNodes removes rows permanently and returns what was removed so the
	// caller can release the stored objects.
	DeleteNodes(context.Context, []uuid.UUID) ([]*Node, error)
	// NextAvailableName returns a sibling name that does not collide yet.
	NextAvailableName(context.Context, uuid.UUID, string) (string, error)
	SubtreeStats(context.Context, uuid.UUID, bool) (*NodeStats, error)
	CategorySizes(context.Context, uuid.UUID, bool) (map[string]int64, error)
}

// AclRepo persists node access control entries.
type AclRepo interface {
	ListAcl(context.Context, uuid.UUID) ([]*AclEntry, error)
	ListAclForNodes(context.Context, []uuid.UUID) (map[uuid.UUID][]*AclEntry, error)
	ReplaceAcl(context.Context, uuid.UUID, []*AclEntry) error
	DeleteAclForNodes(context.Context, []uuid.UUID) error
}

// PasswordVerifier compares a clear text password against a stored hash.
type PasswordVerifier interface {
	Verify(hash, password string) error
}

// NodeStats summarises a subtree.
type NodeStats struct {
	NodeID          uuid.UUID
	FileCount       int64
	FolderCount     int64
	TotalSize       int64
	TrashedCount    int64
	TrashedSize     int64
	LargestFileSize int64
	LargestFileName string
	CategorySizes   map[string]int64
}

// Outcome is a node paired with the access the caller holds on it.
type Outcome struct {
	Node   *Node
	Access NodeAccess
}

// NodeUsecaseOptions carries the tree limits taken from configuration.
type NodeUsecaseOptions struct {
	MaxDepth             int32
	MaxNameLength        int
	CaseInsensitiveNames bool
	RootName             string
	DefaultVisibility    Visibility
}

// NodeUsecase implements the tree operations and the access rules.
type NodeUsecase struct {
	repo     NodeRepo
	acl      AclRepo
	users    UserRepo
	files    FileRepo
	tx       TxManager
	hash     PasswordVerifier
	maxDepth int32
	maxName  int
	caseFold bool
	rootName string
	defVis   Visibility
}

// NewNodeUsecase returns a node usecase.
func NewNodeUsecase(repo NodeRepo, acl AclRepo, users UserRepo, files FileRepo, tx TxManager, hash PasswordVerifier, opts NodeUsecaseOptions) *NodeUsecase {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 32
	}
	if opts.MaxNameLength <= 0 {
		opts.MaxNameLength = 255
	}
	if opts.RootName == "" {
		opts.RootName = "我的网盘"
	}
	if opts.DefaultVisibility == VisibilityUnspecified {
		opts.DefaultVisibility = VisibilityPrivate
	}
	return &NodeUsecase{
		repo:     repo,
		acl:      acl,
		users:    users,
		files:    files,
		tx:       tx,
		hash:     hash,
		maxDepth: opts.MaxDepth,
		maxName:  opts.MaxNameLength,
		caseFold: opts.CaseInsensitiveNames,
		rootName: opts.RootName,
		defVis:   opts.DefaultVisibility,
	}
}

// RootName is the display name of the virtual root.
func (uc *NodeUsecase) RootName() string { return uc.rootName }

// MaxNameLength is the maximum accepted node name length.
func (uc *NodeUsecase) MaxNameLength() int { return uc.maxName }

// MaxDepth is the maximum accepted nesting depth.
func (uc *NodeUsecase) MaxDepth() int32 { return uc.maxDepth }

// CreateFolderInput describes a new folder.
type CreateFolderInput struct {
	ParentID     uuid.UUID
	Name         string
	Description  string
	Visibility   Visibility
	PasswordHash string
	PasswordHint string
	Metadata     map[string]string
	Policy       ConflictPolicy
	Acl          []*AclEntry
}

// AccessChain is the pre-loaded context needed to evaluate node access.
type AccessChain struct {
	Ancestors []*Node
	Acl       map[uuid.UUID][]*AclEntry
}

// Chain loads the ancestor chain of a node together with the ACL entries of
// every node on it.
func (uc *NodeUsecase) Chain(ctx context.Context, node *Node) (*AccessChain, error) {
	chain := &AccessChain{Acl: map[uuid.UUID][]*AclEntry{}}
	if node == nil {
		return chain, nil
	}
	ids := []uuid.UUID{node.ID}
	cur := node
	for i := int32(0); i < uc.maxDepth && cur.ParentID != uuid.Nil; i++ {
		parent, err := uc.repo.FindNodeByID(ctx, cur.ParentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				break
			}
			return nil, err
		}
		chain.Ancestors = append([]*Node{parent}, chain.Ancestors...)
		ids = append(ids, parent.ID)
		cur = parent
	}
	acl, err := uc.acl.ListAclForNodes(ctx, ids)
	if err != nil {
		return nil, err
	}
	chain.Acl = acl
	return chain, nil
}

// Evaluate returns the caller's access to a node.
func (uc *NodeUsecase) Evaluate(ctx context.Context, caller *Caller, node *Node) (NodeAccess, error) {
	chain, err := uc.Chain(ctx, node)
	if err != nil {
		return NodeAccess{}, err
	}
	return EvaluateNodeAccess(NodeAccessInput{
		Caller:    caller,
		Node:      node,
		Ancestors: chain.Ancestors,
		AclByNode: chain.Acl,
	}), nil
}

// EvaluateNodes returns the caller's access to several children of the same
// parent. The shared ancestor chain and the child ACL are loaded once, so a
// listing costs two extra queries regardless of its size.
func (uc *NodeUsecase) EvaluateNodes(ctx context.Context, caller *Caller, parent *Node, children []*Node) (map[uuid.UUID]NodeAccess, error) {
	var ancestors []*Node
	if parent != nil {
		chain, err := uc.Chain(ctx, parent)
		if err != nil {
			return nil, err
		}
		ancestors = append(chain.Ancestors, parent)
	}
	ids := make([]uuid.UUID, 0, len(children))
	for _, c := range children {
		ids = append(ids, c.ID)
	}
	acl, err := uc.acl.ListAclForNodes(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]NodeAccess, len(children))
	for _, c := range children {
		out[c.ID] = EvaluateNodeAccess(NodeAccessInput{
			Caller:    caller,
			Node:      c,
			Ancestors: ancestors,
			AclByNode: acl,
		})
	}
	return out, nil
}

// RequireAccess loads a node and fails unless the caller holds want on it. A
// trashed node is invisible unless the caller may manage the trash.
func (uc *NodeUsecase) RequireAccess(ctx context.Context, caller *Caller, id uuid.UUID, want PermMask) (*Node, NodeAccess, error) {
	node, err := uc.repo.FindNodeByID(ctx, id)
	if err != nil {
		return nil, NodeAccess{}, err
	}
	if node.Status == NodeStatusTrashed && !caller.Has(PermTrashManage) {
		return nil, NodeAccess{}, ErrNotFound
	}
	access, err := uc.Evaluate(ctx, caller, node)
	if err != nil {
		return nil, NodeAccess{}, err
	}
	if access.Locked {
		return node, access, ErrNodeLocked
	}
	if !access.Perms.Has(want) {
		return node, access, ErrPermissionDenied
	}
	return node, access, nil
}

// ListNodesInput describes a listing request.
type ListNodesInput struct {
	ParentID       uuid.UUID
	Recursive      bool
	MaxDepth       int32
	IncludeTrashed bool
}

// ListNodes lists the children of a folder, or the caller's root level when
// ParentID is empty. Children the caller may not see are dropped, and listing
// a locked folder is refused, which is what makes the folder password
// meaningful.
func (uc *NodeUsecase) ListNodes(ctx context.Context, in ListNodesInput, opts ...ListOption) ([]Outcome, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	var parent *Node
	if in.ParentID != uuid.Nil {
		p, err := uc.repo.FindNodeByID(ctx, in.ParentID)
		if err != nil {
			return nil, err
		}
		access, err := uc.Evaluate(ctx, caller, p)
		if err != nil {
			return nil, err
		}
		if access.Locked {
			return nil, ErrNodeLocked
		}
		if !access.Perms.Has(PermView) {
			return nil, ErrPermissionDenied
		}
		parent = p
	}

	query := NodeQuery{ParentID: &in.ParentID, IncludeTrashed: in.IncludeTrashed && caller.Has(PermTrashManage)}
	if in.IncludeTrashed {
		query.Status = nil
	}
	if in.Recursive && parent != nil {
		query.ParentID = nil
		query.PathPrefix = parent.Path
		query.MaxDepth = in.MaxDepth
		if query.MaxDepth <= 0 {
			query.MaxDepth = uc.maxDepth - parent.Depth
		}
		query.MaxDepth += parent.Depth
	}
	nodes, err := uc.repo.ListNodes(ctx, query, opts...)
	if err != nil {
		return nil, err
	}
	accesses, err := uc.EvaluateNodes(ctx, caller, parent, nodes)
	if err != nil {
		return nil, err
	}
	out := make([]Outcome, 0, len(nodes))
	for _, n := range nodes {
		a := accesses[n.ID]
		if !a.Locked && !a.Perms.Has(PermView) {
			continue
		}
		out = append(out, Outcome{Node: n, Access: a})
	}
	return out, nil
}

// CountNodes counts the visible children of a folder.
func (uc *NodeUsecase) CountNodes(ctx context.Context, in ListNodesInput) (int64, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return 0, ErrUnauthenticated
	}
	query := NodeQuery{ParentID: &in.ParentID, IncludeTrashed: in.IncludeTrashed && caller.Has(PermTrashManage)}
	if in.ParentID != uuid.Nil {
		if _, _, err := uc.requireView(ctx, caller, in.ParentID); err != nil {
			return 0, err
		}
	}
	return uc.repo.CountNodes(ctx, query)
}

// GetNode returns one node the caller may see.
func (uc *NodeUsecase) GetNode(ctx context.Context, id uuid.UUID) (*Node, NodeAccess, error) {
	caller := CallerFromContext(ctx)
	if id == uuid.Nil {
		return nil, NodeAccess{}, ErrInvalidArgument
	}
	if !caller.IsAuthenticated() {
		return nil, NodeAccess{}, ErrUnauthenticated
	}
	node, err := uc.repo.FindNodeByID(ctx, id)
	if err != nil {
		return nil, NodeAccess{}, err
	}
	access, err := uc.Evaluate(ctx, caller, node)
	if err != nil {
		return nil, NodeAccess{}, err
	}
	if access.Perms.IsZero() {
		return nil, access, ErrPermissionDenied
	}
	return node, access, nil
}

// Path returns the ancestor chain of a node from the root down.
func (uc *NodeUsecase) Path(ctx context.Context, id uuid.UUID) ([]*Node, *Node, error) {
	node, _, err := uc.GetNode(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	chain, err := uc.Chain(ctx, node)
	if err != nil {
		return nil, nil, err
	}
	return chain.Ancestors, node, nil
}

// DisplayPath renders the human readable absolute path of a node.
func (uc *NodeUsecase) DisplayPath(ctx context.Context, node *Node) (string, error) {
	chain, err := uc.Chain(ctx, node)
	if err != nil {
		return "", err
	}
	return DisplayPathOf(chain.Ancestors, node, uc.rootName), nil
}

// DisplayPathOf joins the ancestor names into an absolute path.
func DisplayPathOf(ancestors []*Node, node *Node, rootName string) string {
	if rootName == "" {
		rootName = "我的网盘"
	}
	parts := make([]string, 0, len(ancestors)+2)
	parts = append(parts, rootName)
	for _, a := range ancestors {
		parts = append(parts, a.Name)
	}
	if node != nil {
		parts = append(parts, node.Name)
	}
	return "/" + path.Join(parts...)
}

// CreateFolder creates a folder with its ACL in a single transaction.
func (uc *NodeUsecase) CreateFolder(ctx context.Context, in CreateFolderInput) (*Node, NodeAccess, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, NodeAccess{}, ErrUnauthenticated
	}
	if err := caller.Require(PermUpload); err != nil {
		return nil, NodeAccess{}, err
	}
	name, err := uc.normalizeName(in.Name)
	if err != nil {
		return nil, NodeAccess{}, err
	}
	visibility := in.Visibility
	if visibility == VisibilityUnspecified {
		visibility = uc.defVis
	}
	var created *Node
	err = uc.tx.WithTx(ctx, func(ctx context.Context) error {
		parent, err := uc.loadWritableParent(ctx, caller, in.ParentID)
		if err != nil {
			return err
		}
		if parent.Depth+1 >= uc.maxDepth {
			return ErrInvalidArgument
		}
		finalName, err := uc.resolveName(ctx, in.ParentID, name, in.Policy, uuid.Nil)
		if err != nil {
			return err
		}
		id := NewID()
		folder := &Node{
			ID:           id,
			ParentID:     in.ParentID,
			Name:         finalName,
			NameLower:    uc.foldName(finalName),
			Kind:         NodeKindFolder,
			OwnerID:      caller.UserID,
			Status:       NodeStatusActive,
			Description:  in.Description,
			Visibility:   visibility,
			PasswordHash: in.PasswordHash,
			PasswordHint: in.PasswordHint,
			Metadata:     in.Metadata,
			Path:         ChildPath(parent.Path, id),
			Depth:        parent.Depth + 1,
			CreatedBy:    caller.UserID,
			UpdatedBy:    caller.UserID,
		}
		saved, err := uc.repo.CreateNode(ctx, folder)
		if err != nil {
			return err
		}
		if len(in.Acl) > 0 {
			entries, err := uc.prepareAcl(ctx, caller, saved.ID, in.Acl)
			if err != nil {
				return err
			}
			if err := uc.acl.ReplaceAcl(ctx, saved.ID, entries); err != nil {
				return err
			}
		}
		if err := uc.users.AddUsage(ctx, caller.UserID, 0, 0, 1); err != nil {
			return err
		}
		created = saved
		return nil
	})
	if err != nil {
		return nil, NodeAccess{}, err
	}
	return created, NodeAccess{Perms: PermNodeScope.And(caller.Perms), Owned: true, Visibility: created.Visibility}, nil
}

// UpdateNodeInput describes a partial node update.
type UpdateNodeInput struct {
	ID            uuid.UUID
	Name          *string
	Description   *string
	Visibility    *Visibility
	Metadata      map[string]string
	ParentID      *uuid.UUID
	PasswordHash  *string
	PasswordHint  *string
	ClearPassword bool
	Policy        ConflictPolicy
}

// UpdateNode applies a partial update after re-checking the node permissions.
func (uc *NodeUsecase) UpdateNode(ctx context.Context, in UpdateNodeInput) (*Node, NodeAccess, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, NodeAccess{}, ErrUnauthenticated
	}
	if in.ID == uuid.Nil {
		return nil, NodeAccess{}, ErrInvalidArgument
	}
	var out *Node
	var outAccess NodeAccess
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		node, err := uc.repo.FindNodeByID(ctx, in.ID)
		if err != nil {
			return err
		}
		access, err := uc.Evaluate(ctx, caller, node)
		if err != nil {
			return err
		}
		if access.Locked {
			return ErrNodeLocked
		}
		aclChange := in.Visibility != nil || in.PasswordHash != nil || in.ClearPassword || in.PasswordHint != nil
		if aclChange && !access.Perms.Has(PermAclManage) {
			return ErrPermissionDenied
		}
		contentChange := in.Name != nil || in.ParentID != nil || in.Description != nil || in.Metadata != nil
		if contentChange && !access.Perms.Has(PermEdit) {
			return ErrPermissionDenied
		}
		if in.Name != nil {
			name, err := uc.normalizeName(*in.Name)
			if err != nil {
				return err
			}
			if name != node.Name {
				finalName, err := uc.resolveName(ctx, node.ParentID, name, in.Policy, node.ID)
				if err != nil {
					return err
				}
				if err := uc.repo.RenameNode(ctx, node.ID, finalName); err != nil {
					return err
				}
				node.Name, node.NameLower = finalName, uc.foldName(finalName)
			}
		}
		if in.ParentID != nil && *in.ParentID != node.ParentID {
			if err := uc.relocate(ctx, caller, node, *in.ParentID, in.Policy); err != nil {
				return err
			}
		}
		if in.Description != nil {
			node.Description = *in.Description
		}
		if in.Metadata != nil {
			node.Metadata = in.Metadata
		}
		if in.Visibility != nil {
			node.Visibility = *in.Visibility
		}
		if in.PasswordHash != nil {
			node.PasswordHash = *in.PasswordHash
		}
		if in.PasswordHint != nil {
			node.PasswordHint = *in.PasswordHint
		}
		if in.ClearPassword {
			node.PasswordHash, node.PasswordHint = "", ""
		}
		node.UpdatedBy = caller.UserID
		saved, err := uc.repo.UpdateNode(ctx, node)
		if err != nil {
			return err
		}
		out = saved
		outAccess, err = uc.Evaluate(ctx, caller, saved)
		return err
	})
	if err != nil {
		return nil, NodeAccess{}, err
	}
	return out, outAccess, nil
}

// MoveNodesInput describes a batch move.
type MoveNodesInput struct {
	IDs            []uuid.UUID
	TargetParentID uuid.UUID
	Policy         ConflictPolicy
	NewNames       []string
}

// MoveNodes moves a batch atomically. A single rejected node rolls every
// earlier move back, so dragging a selection either fully succeeds or leaves
// the tree untouched.
func (uc *NodeUsecase) MoveNodes(ctx context.Context, in MoveNodesInput) ([]Outcome, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	if len(in.IDs) == 0 {
		return nil, ErrInvalidArgument
	}
	if len(in.NewNames) > 0 && len(in.NewNames) != len(in.IDs) {
		return nil, ErrInvalidArgument
	}
	var outcomes []Outcome
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		outcomes = outcomes[:0]
		for i, id := range in.IDs {
			node, err := uc.repo.FindNodeByID(ctx, id)
			if err != nil {
				return err
			}
			access, err := uc.Evaluate(ctx, caller, node)
			if err != nil {
				return err
			}
			if access.Locked {
				return ErrNodeLocked
			}
			if !access.Perms.Has(PermEdit) {
				return ErrPermissionDenied
			}
			if err := uc.relocate(ctx, caller, node, in.TargetParentID, in.Policy); err != nil {
				return err
			}
			if len(in.NewNames) > 0 && in.NewNames[i] != "" {
				name, err := uc.normalizeName(in.NewNames[i])
				if err != nil {
					return err
				}
				finalName, err := uc.resolveName(ctx, in.TargetParentID, name, in.Policy, node.ID)
				if err != nil {
					return err
				}
				if err := uc.repo.RenameNode(ctx, node.ID, finalName); err != nil {
					return err
				}
			}
			moved, err := uc.repo.FindNodeByID(ctx, id)
			if err != nil {
				return err
			}
			a, err := uc.Evaluate(ctx, caller, moved)
			if err != nil {
				return err
			}
			outcomes = append(outcomes, Outcome{Node: moved, Access: a})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return outcomes, nil
}

// relocate moves one node under a new parent after checking the permissions,
// the depth limit and the cycle condition.
func (uc *NodeUsecase) relocate(ctx context.Context, caller *Caller, node *Node, targetParentID uuid.UUID, policy ConflictPolicy) error {
	if targetParentID == node.ID {
		return ErrCycleDetected
	}
	if targetParentID == node.ParentID {
		return nil
	}
	parent, err := uc.loadWritableParent(ctx, caller, targetParentID)
	if err != nil {
		return err
	}
	if node.IsFolder() && strings.HasPrefix(parent.Path, node.Path) {
		return ErrCycleDetected
	}
	if parent.Depth+1 >= uc.maxDepth {
		return ErrInvalidArgument
	}
	finalName, err := uc.resolveName(ctx, targetParentID, node.Name, policy, node.ID)
	if err != nil {
		return err
	}
	oldPath := node.Path
	newPath := ChildPath(parent.Path, node.ID)
	delta := parent.Depth + 1 - node.Depth
	if err := uc.repo.RelocateSubtree(ctx, node, targetParentID, newPath, delta); err != nil {
		return err
	}
	node.ParentID = targetParentID
	node.Path = newPath
	node.Depth = parent.Depth + 1
	node.UpdatedBy = caller.UserID
	if finalName != node.Name {
		if err := uc.repo.RenameNode(ctx, node.ID, finalName); err != nil {
			return err
		}
		node.Name, node.NameLower = finalName, uc.foldName(finalName)
	}
	_ = oldPath
	_, err = uc.repo.UpdateNode(ctx, node)
	return err
}

// loadWritableParent resolves a destination folder and checks that the caller
// may write inside it.
func (uc *NodeUsecase) loadWritableParent(ctx context.Context, caller *Caller, parentID uuid.UUID) (*Node, error) {
	if parentID == uuid.Nil {
		return &Node{ID: uuid.Nil, Path: "/", Depth: -1}, nil
	}
	parent, err := uc.repo.FindNodeByID(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if parent.Kind != NodeKindFolder {
		return nil, ErrInvalidArgument
	}
	if parent.Status == NodeStatusTrashed {
		return nil, ErrInvalidArgument
	}
	access, err := uc.Evaluate(ctx, caller, parent)
	if err != nil {
		return nil, err
	}
	if access.Locked {
		return nil, ErrNodeLocked
	}
	if !access.Perms.Has(PermUpload) {
		return nil, ErrPermissionDenied
	}
	return parent, nil
}

// DeleteNodesInput describes a delete batch.
type DeleteNodesInput struct {
	IDs       []uuid.UUID
	Permanent bool
}

// DeleteNodes moves nodes to the trash, or purges them when Permanent is set.
func (uc *NodeUsecase) DeleteNodes(ctx context.Context, in DeleteNodesInput) ([]uuid.UUID, int64, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, 0, ErrUnauthenticated
	}
	if len(in.IDs) == 0 {
		return nil, 0, ErrInvalidArgument
	}
	if in.Permanent {
		ids, reclaimed, err := uc.PurgeNodes(ctx, in.IDs)
		return ids, reclaimed, err
	}
	var ids []uuid.UUID
	var affected int64
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		ids = ids[:0]
		affected = 0
		now := time.Now()
		for _, id := range in.IDs {
			node, err := uc.repo.FindNodeByID(ctx, id)
			if err != nil {
				return err
			}
			access, err := uc.Evaluate(ctx, caller, node)
			if err != nil {
				return err
			}
			if access.Locked {
				return ErrNodeLocked
			}
			if !access.Perms.Has(PermDelete) {
				return ErrPermissionDenied
			}
			n, err := uc.repo.SetSubtreeStatus(ctx, node, NodeStatusTrashed, &now)
			if err != nil {
				return err
			}
			affected += n
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return ids, affected, nil
}

// RestoreNodesInput describes a restore batch.
type RestoreNodesInput struct {
	IDs            []uuid.UUID
	TargetParentID uuid.UUID
	Policy         ConflictPolicy
}

// RestoreNodes restores nodes from the trash.
func (uc *NodeUsecase) RestoreNodes(ctx context.Context, in RestoreNodesInput) ([]Outcome, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	if len(in.IDs) == 0 {
		return nil, ErrInvalidArgument
	}
	var outcomes []Outcome
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		outcomes = outcomes[:0]
		for _, id := range in.IDs {
			node, err := uc.repo.FindNodeByID(ctx, id)
			if err != nil {
				return err
			}
			if node.Status != NodeStatusTrashed {
				return ErrInvalidArgument
			}
			access, err := uc.Evaluate(ctx, caller, node)
			if err != nil {
				return err
			}
			if !access.Owned && !access.Perms.Has(PermTrashManage) {
				return ErrPermissionDenied
			}
			target := in.TargetParentID
			if target == uuid.Nil {
				target = node.OriginalParentID
			}
			if target != uuid.Nil {
				if _, err := uc.repo.FindNodeByID(ctx, target); err != nil {
					// The original parent is gone: fall back to the root.
					target = uuid.Nil
				}
			}
			parentPath, parentDepth, err := uc.parentLocation(ctx, target)
			if err != nil {
				return err
			}
			if parentDepth+1 >= uc.maxDepth {
				return ErrInvalidArgument
			}
			finalName, err := uc.resolveName(ctx, target, node.Name, in.Policy, node.ID)
			if err != nil {
				return err
			}
			newPath := ChildPath(parentPath, node.ID)
			if err := uc.repo.RelocateSubtree(ctx, node, target, newPath, parentDepth+1-node.Depth); err != nil {
				return err
			}
			node.ParentID = target
			node.Path = newPath
			node.Depth = parentDepth + 1
			node.OriginalParentID = uuid.Nil
			node.UpdatedBy = caller.UserID
			if finalName != node.Name {
				if err := uc.repo.RenameNode(ctx, node.ID, finalName); err != nil {
					return err
				}
				node.Name, node.NameLower = finalName, uc.foldName(finalName)
			}
			if _, err := uc.repo.SetSubtreeStatus(ctx, node, NodeStatusActive, nil); err != nil {
				return err
			}
			saved, err := uc.repo.FindNodeByID(ctx, node.ID)
			if err != nil {
				return err
			}
			a, err := uc.Evaluate(ctx, caller, saved)
			if err != nil {
				return err
			}
			outcomes = append(outcomes, Outcome{Node: saved, Access: a})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return outcomes, nil
}

// PurgeNodes permanently deletes nodes and reports the reclaimed bytes. The
// stored objects are released after the transaction commits, so a crash can
// only ever leak an object, never lose a live one.
func (uc *NodeUsecase) PurgeNodes(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, int64, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, 0, ErrUnauthenticated
	}
	if len(ids) == 0 {
		return nil, 0, ErrInvalidArgument
	}
	var purged []uuid.UUID
	var reclaimed int64
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		purged = purged[:0]
		reclaimed = 0
		for _, id := range ids {
			node, err := uc.repo.FindNodeByID(ctx, id)
			if err != nil {
				return err
			}
			access, err := uc.Evaluate(ctx, caller, node)
			if err != nil {
				return err
			}
			if !access.Perms.Has(PermTrashManage) {
				return ErrPermissionDenied
			}
			if node.Status != NodeStatusTrashed {
				return ErrInvalidArgument
			}
			subtree, err := uc.repo.ListNodes(ctx, NodeQuery{PathPrefix: node.Path, IncludeTrashed: true})
			if err != nil {
				return err
			}
			all := make([]uuid.UUID, 0, len(subtree))
			var bytes int64
			var files, folders int64
			for _, n := range subtree {
				all = append(all, n.ID)
				if n.IsFile() {
					bytes += n.Size
					files++
				} else {
					folders++
				}
			}
			removed, err := uc.repo.DeleteNodes(ctx, all)
			if err != nil {
				return err
			}
			if err := uc.acl.DeleteAclForNodes(ctx, all); err != nil {
				return err
			}
			if err := uc.users.AddUsage(ctx, node.OwnerID, -bytes, -files, -folders); err != nil {
				return err
			}
			// The stored objects are queued rather than removed here: the
			// queue is drained after the commit, so a failure can only leave
			// an unreferenced object behind, which the maintenance job
			// reclaims, and never a row pointing at missing content.
			if err := uc.releaseObjects(ctx, removed); err != nil {
				return err
			}
			reclaimed += bytes
			purged = append(purged, id)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return purged, reclaimed, nil
}

// releaseObjects queues the stored content of the supplied nodes, including
// every retained revision, for deletion once the transaction commits. Without
// it a permanent delete would drop the rows and leave the bytes in object
// storage forever.
func (uc *NodeUsecase) releaseObjects(ctx context.Context, removed []*Node) error {
	if uc.files == nil {
		return nil
	}
	for _, n := range removed {
		if n.StorageKey != "" {
			if err := uc.files.EnqueueDeletion(ctx, n.StorageKey, "node_purge"); err != nil {
				return err
			}
		}
		versions, err := uc.files.ListVersions(ctx, n.ID, ListLimit(1000))
		if err != nil {
			return err
		}
		for _, v := range versions {
			if err := uc.files.EnqueueDeletion(ctx, v.StorageKey, "node_purge_version"); err != nil {
				return err
			}
			if err := uc.files.DeleteVersion(ctx, v.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// TrashAllForOwner moves every live node of an account to the trash. It is the
// administrative path used when an account is deleted with its content; there
// is no caller check because the account has already been authorised.
func (uc *NodeUsecase) TrashAllForOwner(ctx context.Context, ownerID uuid.UUID) (int64, error) {
	if ownerID == uuid.Nil {
		return 0, ErrInvalidArgument
	}
	var affected int64
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		affected = 0
		status := NodeStatusActive
		owner := ownerID
		roots, err := uc.repo.ListNodes(ctx, NodeQuery{
			Roots: true, OwnerID: &owner, Status: &status,
		}, ListLimit(10000))
		if err != nil {
			return err
		}
		now := time.Now()
		for _, node := range roots {
			n, err := uc.repo.SetSubtreeStatus(ctx, node, NodeStatusTrashed, &now)
			if err != nil {
				return err
			}
			affected += n
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// TrashNodes lists the trash visible to the caller.
func (uc *NodeUsecase) TrashNodes(ctx context.Context, originalParent uuid.UUID, opts ...ListOption) ([]Outcome, int64, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, 0, ErrUnauthenticated
	}
	status := NodeStatusTrashed
	query := NodeQuery{Status: &status, IncludeTrashed: true}
	if originalParent != uuid.Nil {
		query.ParentID = &originalParent
	}
	if !caller.Has(PermTrashManage) {
		owner := caller.UserID
		query.OwnerID = &owner
	}
	nodes, err := uc.repo.ListNodes(ctx, query, opts...)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.repo.CountNodes(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Outcome, 0, len(nodes))
	for _, n := range nodes {
		a, err := uc.Evaluate(ctx, caller, n)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, Outcome{Node: n, Access: a})
	}
	return out, total, nil
}

// EmptyTrash purges every trashed node the caller may manage.
func (uc *NodeUsecase) EmptyTrash(ctx context.Context, before *time.Time) ([]uuid.UUID, int64, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, 0, ErrUnauthenticated
	}
	status := NodeStatusTrashed
	query := NodeQuery{Status: &status, IncludeTrashed: true}
	if !caller.Has(PermTrashManage) {
		owner := caller.UserID
		query.OwnerID = &owner
	}
	nodes, err := uc.repo.ListNodes(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, 0, len(nodes))
	for _, n := range nodes {
		if before != nil && (n.TrashedAt == nil || n.TrashedAt.After(*before)) {
			continue
		}
		ids = append(ids, n.ID)
	}
	if len(ids) == 0 {
		return nil, 0, nil
	}
	return uc.PurgeNodes(ctx, ids)
}

// SetNodeAclInput describes an ACL replacement.
type SetNodeAclInput struct {
	NodeID    uuid.UUID
	Entries   []*AclEntry
	Recursive bool
}

// SetNodeAcl replaces the ACL of a node and optionally of its whole subtree.
func (uc *NodeUsecase) SetNodeAcl(ctx context.Context, in SetNodeAclInput) ([]*AclEntry, NodeAccess, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, NodeAccess{}, ErrUnauthenticated
	}
	if in.NodeID == uuid.Nil {
		return nil, NodeAccess{}, ErrInvalidArgument
	}
	for _, e := range in.Entries {
		if e.Permissions.And(^PermNodeScope) != 0 {
			return nil, NodeAccess{}, ErrInvalidArgument
		}
		if !caller.Perms.Has(e.Permissions) {
			return nil, NodeAccess{}, ErrPermissionDenied
		}
		switch e.SubjectType {
		case SubjectTypeEveryone:
		case SubjectTypeUser, SubjectTypeRole:
			if strings.TrimSpace(e.SubjectID) == "" {
				return nil, NodeAccess{}, ErrInvalidArgument
			}
		default:
			return nil, NodeAccess{}, ErrInvalidArgument
		}
		if e.Effect != EffectAllow && e.Effect != EffectDeny {
			return nil, NodeAccess{}, ErrInvalidArgument
		}
	}
	var entries []*AclEntry
	var access NodeAccess
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		node, err := uc.repo.FindNodeByID(ctx, in.NodeID)
		if err != nil {
			return err
		}
		a, err := uc.Evaluate(ctx, caller, node)
		if err != nil {
			return err
		}
		if !a.Perms.Has(PermAclManage) {
			return ErrPermissionDenied
		}
		prepared, err := uc.prepareAcl(ctx, caller, in.NodeID, in.Entries)
		if err != nil {
			return err
		}
		if err := uc.acl.ReplaceAcl(ctx, in.NodeID, prepared); err != nil {
			return err
		}
		if in.Recursive && node.IsFolder() {
			subtree, err := uc.repo.ListNodes(ctx, NodeQuery{
				PathPrefix: node.Path,
				ExcludeIDs: []uuid.UUID{node.ID},
			})
			if err != nil {
				return err
			}
			for _, child := range subtree {
				cloned, err := uc.prepareAcl(ctx, caller, child.ID, in.Entries)
				if err != nil {
					return err
				}
				if err := uc.acl.ReplaceAcl(ctx, child.ID, cloned); err != nil {
					return err
				}
			}
		}
		entries, err = uc.acl.ListAcl(ctx, in.NodeID)
		if err != nil {
			return err
		}
		access, err = uc.Evaluate(ctx, caller, node)
		return err
	})
	if err != nil {
		return nil, NodeAccess{}, err
	}
	return entries, access, nil
}

// prepareAcl normalises incoming entries for storage on a node.
func (uc *NodeUsecase) prepareAcl(ctx context.Context, caller *Caller, nodeID uuid.UUID, in []*AclEntry) ([]*AclEntry, error) {
	out := make([]*AclEntry, 0, len(in))
	for _, e := range in {
		copied := *e
		copied.ID = uuid.Nil
		copied.NodeID = nodeID
		copied.CreatedBy = caller.UserID
		copied.Permissions = copied.Permissions.And(PermNodeScope)
		if !caller.Perms.Has(copied.Permissions) {
			return nil, ErrPermissionDenied
		}
		out = append(out, &copied)
	}
	return out, nil
}

// Acl returns the ACL of a node together with the entries it inherits.
func (uc *NodeUsecase) Acl(ctx context.Context, id uuid.UUID) ([]*AclEntry, []*AclEntry, NodeAccess, error) {
	caller := CallerFromContext(ctx)
	node, err := uc.repo.FindNodeByID(ctx, id)
	if err != nil {
		return nil, nil, NodeAccess{}, err
	}
	chain, err := uc.Chain(ctx, node)
	if err != nil {
		return nil, nil, NodeAccess{}, err
	}
	access := EvaluateNodeAccess(NodeAccessInput{
		Caller: caller, Node: node, Ancestors: chain.Ancestors, AclByNode: chain.Acl,
	})
	if access.Perms.IsZero() {
		return nil, nil, access, ErrPermissionDenied
	}
	inherited := make([]*AclEntry, 0)
	for _, a := range chain.Ancestors {
		for _, e := range chain.Acl[a.ID] {
			if e.Effect == EffectDeny || e.Inherit {
				inherited = append(inherited, e)
			}
		}
	}
	return chain.Acl[node.ID], inherited, access, nil
}

// Unlock verifies a folder password and returns the node the password belongs
// to, which is the outermost protected ancestor.
func (uc *NodeUsecase) Unlock(ctx context.Context, id uuid.UUID, password string) (*Node, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidArgument
	}
	node, err := uc.repo.FindNodeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	chain, err := uc.Chain(ctx, node)
	if err != nil {
		return nil, err
	}
	target := node
	for _, a := range chain.Ancestors {
		if a.PasswordHash != "" {
			target = a
			break
		}
	}
	if target.PasswordHash == "" {
		return target, nil
	}
	if password == "" {
		return nil, ErrNodeLocked
	}
	if uc.hash == nil || uc.hash.Verify(target.PasswordHash, password) != nil {
		return nil, ErrPermissionDenied
	}
	return target, nil
}

// SearchNodesInput describes a tree-wide search.
type SearchNodesInput struct {
	Query            string
	ScopeNodeID      uuid.UUID
	Kind             NodeKind
	MimeType         string
	OwnerID          uuid.UUID
	MinSize          int64
	MaxSize          int64
	UpdatedAfter     *time.Time
	UpdatedBefore    *time.Time
	MatchDescription bool
}

// searchScanLimit bounds how many rows a search inspects before paginating, so
// a tree-wide query cannot stall the process.
const searchScanLimit = 5000

// SearchNodes searches the tree for nodes the caller may see.
func (uc *NodeUsecase) SearchNodes(ctx context.Context, in SearchNodesInput, opts ...ListOption) ([]Outcome, int64, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, 0, ErrUnauthenticated
	}
	listOpts := ResolveListOptions(50, opts...)
	query := NodeQuery{}
	if in.ScopeNodeID != uuid.Nil {
		scope, _, err := uc.requireView(ctx, caller, in.ScopeNodeID)
		if err != nil {
			return nil, 0, err
		}
		query.PathPrefix = scope.Path
	}
	if in.Kind != NodeKindUnspecified {
		kind := in.Kind
		query.Kind = &kind
	}
	if in.OwnerID != uuid.Nil {
		owner := in.OwnerID
		query.OwnerID = &owner
	}
	nodes, err := uc.repo.ListNodes(ctx, query, ListLimit(searchScanLimit))
	if err != nil {
		return nil, 0, err
	}
	needle := strings.ToLower(strings.TrimSpace(in.Query))
	matched := make([]Outcome, 0, len(nodes))
	for _, n := range nodes {
		if needle != "" {
			haystack := strings.ToLower(n.Name)
			if in.MatchDescription {
				haystack = strings.ToLower(n.Description)
			}
			if !strings.Contains(haystack, needle) {
				continue
			}
		}
		if in.MimeType != "" && !strings.HasPrefix(n.MimeType, in.MimeType) {
			continue
		}
		if in.MinSize > 0 && n.Size < in.MinSize {
			continue
		}
		if in.MaxSize > 0 && n.Size > in.MaxSize {
			continue
		}
		if in.UpdatedAfter != nil && n.UpdatedAt.Before(*in.UpdatedAfter) {
			continue
		}
		if in.UpdatedBefore != nil && n.UpdatedAt.After(*in.UpdatedBefore) {
			continue
		}
		a, err := uc.Evaluate(ctx, caller, n)
		if err != nil {
			return nil, 0, err
		}
		if !a.Perms.Has(PermView) || a.Locked {
			continue
		}
		matched = append(matched, Outcome{Node: n, Access: a})
	}
	total := int64(len(matched))
	start := listOpts.Offset
	if start > len(matched) {
		start = len(matched)
	}
	end := start + listOpts.Limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[start:end], total, nil
}

// requireView loads a node and fails unless the caller may view it.
func (uc *NodeUsecase) requireView(ctx context.Context, caller *Caller, id uuid.UUID) (*Node, NodeAccess, error) {
	node, err := uc.repo.FindNodeByID(ctx, id)
	if err != nil {
		return nil, NodeAccess{}, err
	}
	access, err := uc.Evaluate(ctx, caller, node)
	if err != nil {
		return nil, NodeAccess{}, err
	}
	if access.Locked {
		return node, access, ErrNodeLocked
	}
	if !access.Perms.Has(PermView) {
		return nil, access, ErrPermissionDenied
	}
	return node, access, nil
}

// Stats summarises a subtree the caller may see.
func (uc *NodeUsecase) Stats(ctx context.Context, id uuid.UUID) (*NodeStats, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	if id == uuid.Nil {
		stats := &NodeStats{CategorySizes: map[string]int64{}}
		status := NodeStatusActive
		owner := caller.UserID
		nodes, err := uc.repo.ListNodes(ctx, NodeQuery{OwnerID: &owner, Status: &status, IncludeTrashed: true})
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			if n.IsFile() {
				stats.FileCount++
				stats.TotalSize += n.Size
			} else {
				stats.FolderCount++
			}
		}
		return stats, nil
	}
	if _, _, err := uc.requireView(ctx, caller, id); err != nil {
		return nil, err
	}
	stats, err := uc.repo.SubtreeStats(ctx, id, true)
	if err != nil {
		return nil, err
	}
	categories, err := uc.repo.CategorySizes(ctx, id, true)
	if err == nil {
		stats.CategorySizes = categories
	}
	return stats, nil
}

// Tree returns a folder subtree expanded to the requested depth.
func (uc *NodeUsecase) Tree(ctx context.Context, rootID uuid.UUID, depth int32) ([]Outcome, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	if depth <= 0 {
		depth = 1
	}
	if depth > uc.maxDepth {
		depth = uc.maxDepth
	}
	query := NodeQuery{}
	if rootID != uuid.Nil {
		root, _, err := uc.requireView(ctx, caller, rootID)
		if err != nil {
			return nil, err
		}
		query.PathPrefix = root.Path
		query.ExcludeIDs = []uuid.UUID{root.ID}
		query.MaxDepth = root.Depth + depth
	} else {
		owner := caller.UserID
		query.OwnerID = &owner
	}
	nodes, err := uc.repo.ListNodes(ctx, query, ListLimit(5000))
	if err != nil {
		return nil, err
	}
	out := make([]Outcome, 0, len(nodes))
	for _, n := range nodes {
		a, err := uc.Evaluate(ctx, caller, n)
		if err != nil {
			return nil, err
		}
		if !a.Perms.Has(PermView) || a.Locked {
			continue
		}
		out = append(out, Outcome{Node: n, Access: a})
	}
	return out, nil
}

// normalizeName validates and trims a node name.
func (uc *NodeUsecase) normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "", ErrInvalidArgument
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return "", ErrInvalidArgument
	}
	if len(name) > uc.maxName {
		return "", ErrInvalidArgument
	}
	return name, nil
}

func (uc *NodeUsecase) foldName(name string) string {
	if !uc.caseFold {
		return name
	}
	return strings.ToLower(name)
}

// resolveName applies the conflict policy to a name inside a folder. Only live
// siblings are considered, so a trashed node never blocks a new upload. The
// overwrite policy is handled by the upload path, which replaces content
// instead of names; here it is treated like a failure so a rename can never
// silently destroy a sibling.
func (uc *NodeUsecase) resolveName(ctx context.Context, parentID uuid.UUID, name string, policy ConflictPolicy, exclude uuid.UUID) (string, error) {
	query := NodeQuery{ParentID: &parentID, Names: []string{name}}
	if exclude != uuid.Nil {
		query.ExcludeIDs = []uuid.UUID{exclude}
	}
	existing, err := uc.repo.ListNodes(ctx, query, ListLimit(1))
	if err != nil {
		return "", err
	}
	if len(existing) == 0 {
		return name, nil
	}
	if policy == ConflictPolicyRename {
		return uc.repo.NextAvailableName(ctx, parentID, name)
	}
	return "", ErrNameConflict
}

// parentLocation returns the materialised path and depth of a parent folder.
func (uc *NodeUsecase) parentLocation(ctx context.Context, parentID uuid.UUID) (string, int32, error) {
	if parentID == uuid.Nil {
		return "/", -1, nil
	}
	parent, err := uc.repo.FindNodeByID(ctx, parentID)
	if err != nil {
		return "", 0, err
	}
	if parent.Kind != NodeKindFolder || parent.Status == NodeStatusTrashed {
		return "", 0, ErrInvalidArgument
	}
	return parent.Path, parent.Depth, nil
}

// ChildPath returns the materialised path of a child.
func ChildPath(parentPath string, id uuid.UUID) string {
	if parentPath == "" {
		parentPath = "/"
	}
	if !strings.HasSuffix(parentPath, "/") {
		parentPath += "/"
	}
	return parentPath + id.String() + "/"
}
