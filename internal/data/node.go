package data

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/data/ent"
	"nagisa/internal/data/ent/node"
	"nagisa/internal/data/ent/nodeacl"
	"nagisa/internal/data/ent/predicate"

	"github.com/go-kratos/aip-go/ents"
	"github.com/google/uuid"
)

// nodeRepo persists the file tree.
type nodeRepo struct {
	data *Data
}

// NewNodeRepo creates a new NodeRepo instance.
func NewNodeRepo(d *Data) biz.NodeRepo { return &nodeRepo{data: d} }

func toBizNode(po *ent.Node) *biz.Node {
	if po == nil {
		return nil
	}
	return &biz.Node{
		ID:               po.ID,
		ParentID:         po.ParentID,
		Name:             po.Name,
		NameLower:        po.NameLower,
		Kind:             po.Kind,
		OwnerID:          po.OwnerID,
		Size:             po.Size,
		MimeType:         po.MimeType,
		Extension:        po.Extension,
		Etag:             po.Etag,
		StorageKey:       po.StorageKey,
		Status:           po.Status,
		Description:      po.Description,
		Visibility:       po.Visibility,
		PasswordHash:     po.PasswordHash,
		PasswordHint:     po.PasswordHint,
		HasThumbnail:     po.HasThumbnail,
		Metadata:         po.Metadata,
		Path:             po.Path,
		Depth:            po.Depth,
		ChildCount:       po.ChildCount,
		FileCount:        po.FileCount,
		FolderCount:      po.FolderCount,
		SubtreeSize:      po.SubtreeSize,
		CurrentVersionID: po.CurrentVersionID,
		VersionCount:     po.VersionCount,
		TrashedAt:        po.TrashedAt,
		OriginalParentID: po.OriginalParentID,
		CreatedBy:        po.CreatedBy,
		UpdatedBy:        po.UpdatedBy,
		CreatedAt:        po.CreatedAt,
		UpdatedAt:        po.UpdatedAt,
	}
}

// nodePredicates translates a domain query into ent predicates.
func nodePredicates(q biz.NodeQuery) []predicate.Node {
	var ps []predicate.Node
	if q.Roots {
		ps = append(ps, node.ParentIDEQ(uuid.Nil))
	}
	if q.ParentID != nil {
		ps = append(ps, node.ParentIDEQ(*q.ParentID))
	}
	if q.PathPrefix != "" {
		ps = append(ps, node.PathHasPrefix(q.PathPrefix))
	}
	if q.OwnerID != nil {
		ps = append(ps, node.OwnerIDEQ(*q.OwnerID))
	}
	switch {
	case q.Status != nil:
		ps = append(ps, node.StatusEQ(*q.Status))
	case !q.IncludeTrashed:
		ps = append(ps, node.StatusEQ(biz.NodeStatusActive))
	}
	if q.Kind != nil {
		ps = append(ps, node.KindEQ(*q.Kind))
	}
	if len(q.Names) > 0 {
		lowered := make([]string, 0, len(q.Names))
		for _, n := range q.Names {
			lowered = append(lowered, strings.ToLower(n))
		}
		ps = append(ps, node.NameLowerIn(lowered...))
	}
	if len(q.ExcludeIDs) > 0 {
		ps = append(ps, node.IDNotIn(q.ExcludeIDs...))
	}
	if q.MaxDepth > 0 {
		ps = append(ps, node.DepthLTE(q.MaxDepth))
	}
	return ps
}

func (r *nodeRepo) FindNodeByID(ctx context.Context, id uuid.UUID) (*biz.Node, error) {
	po, err := r.data.Exec(ctx).Node().Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find node: %w", err)
	}
	return toBizNode(po), nil
}

func (r *nodeRepo) FindNodesByIDs(ctx context.Context, ids []uuid.UUID) ([]*biz.Node, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	pos, err := r.data.Exec(ctx).Node().Query().Where(node.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: find nodes: %w", err)
	}
	return toBizNodes(pos), nil
}

func (r *nodeRepo) ListNodes(ctx context.Context, q biz.NodeQuery, opts ...biz.ListOption) ([]*biz.Node, error) {
	options := biz.ResolveListOptions(50, opts...)
	query := r.data.Exec(ctx).Node().Query().
		Where(nodePredicates(q)...).
		Where(ents.ApplyFilter(options.Filter)).
		Order(ents.ApplyOrderBy(options.OrderBy), node.ByID()).
		Offset(options.Offset).
		Limit(options.Limit)
	pos, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list nodes: %w", err)
	}
	return toBizNodes(pos), nil
}

func (r *nodeRepo) CountNodes(ctx context.Context, q biz.NodeQuery, opts ...biz.ListOption) (int64, error) {
	options := biz.ResolveListOptions(50, opts...)
	count, err := r.data.Exec(ctx).Node().Query().
		Where(nodePredicates(q)...).
		Where(ents.ApplyFilter(options.Filter)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: count nodes: %w", err)
	}
	return int64(count), nil
}

func (r *nodeRepo) CreateNode(ctx context.Context, n *biz.Node) (*biz.Node, error) {
	builder := r.data.Exec(ctx).Node().Create().
		SetParentID(n.ParentID).
		SetName(n.Name).
		SetNameLower(n.NameLower).
		SetKind(n.Kind).
		SetOwnerID(n.OwnerID).
		SetSize(n.Size).
		SetMimeType(n.MimeType).
		SetExtension(n.Extension).
		SetEtag(n.Etag).
		SetStorageKey(n.StorageKey).
		SetStatus(n.Status).
		SetDescription(n.Description).
		SetVisibility(n.Visibility).
		SetPasswordHash(n.PasswordHash).
		SetPasswordHint(n.PasswordHint).
		SetHasThumbnail(n.HasThumbnail).
		SetPath(n.Path).
		SetDepth(n.Depth).
		SetChildCount(n.ChildCount).
		SetFileCount(n.FileCount).
		SetFolderCount(n.FolderCount).
		SetSubtreeSize(n.SubtreeSize).
		SetVersionCount(n.VersionCount).
		SetOriginalParentID(n.OriginalParentID).
		SetCreatedBy(n.CreatedBy).
		SetUpdatedBy(n.UpdatedBy)
	if n.ID != uuid.Nil {
		builder = builder.SetID(n.ID)
	}
	if n.Metadata != nil {
		builder = builder.SetMetadata(n.Metadata)
	}
	if n.CurrentVersionID != nil {
		builder = builder.SetCurrentVersionID(*n.CurrentVersionID)
	}
	if n.TrashedAt != nil {
		builder = builder.SetTrashedAt(*n.TrashedAt)
	}
	po, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: create node: %w", err)
	}
	return toBizNode(po), nil
}

func (r *nodeRepo) UpdateNode(ctx context.Context, n *biz.Node) (*biz.Node, error) {
	builder := r.data.Exec(ctx).Node().UpdateOneID(n.ID).
		SetParentID(n.ParentID).
		SetName(n.Name).
		SetNameLower(n.NameLower).
		SetKind(n.Kind).
		SetSize(n.Size).
		SetMimeType(n.MimeType).
		SetExtension(n.Extension).
		SetEtag(n.Etag).
		SetStorageKey(n.StorageKey).
		SetStatus(n.Status).
		SetDescription(n.Description).
		SetVisibility(n.Visibility).
		SetPasswordHash(n.PasswordHash).
		SetPasswordHint(n.PasswordHint).
		SetHasThumbnail(n.HasThumbnail).
		SetPath(n.Path).
		SetDepth(n.Depth).
		SetChildCount(n.ChildCount).
		SetFileCount(n.FileCount).
		SetFolderCount(n.FolderCount).
		SetSubtreeSize(n.SubtreeSize).
		SetVersionCount(n.VersionCount).
		SetOriginalParentID(n.OriginalParentID).
		SetUpdatedBy(n.UpdatedBy)
	if n.Metadata != nil {
		builder = builder.SetMetadata(n.Metadata)
	} else {
		builder = builder.ClearMetadata()
	}
	if n.CurrentVersionID != nil {
		builder = builder.SetCurrentVersionID(*n.CurrentVersionID)
	} else {
		builder = builder.ClearCurrentVersionID()
	}
	if n.TrashedAt != nil {
		builder = builder.SetTrashedAt(*n.TrashedAt)
	} else {
		builder = builder.ClearTrashedAt()
	}
	po, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: update node: %w", err)
	}
	return toBizNode(po), nil
}

func (r *nodeRepo) RenameNode(ctx context.Context, id uuid.UUID, name string) error {
	affected, err := r.data.Exec(ctx).Node().Update().
		Where(node.IDEQ(id)).
		SetName(name).
		SetNameLower(strings.ToLower(name)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("data: rename node: %w", err)
	}
	if affected == 0 {
		return biz.ErrNotFound
	}
	return nil
}

// RelocateSubtree moves a node and rewrites the materialised path and depth of
// its descendants. The rewrite runs row by row inside the caller's transaction,
// which keeps it portable across the SQLite and MySQL dialects.
func (r *nodeRepo) RelocateSubtree(ctx context.Context, n *biz.Node, newParentID uuid.UUID, newPath string, depthDelta int32) error {
	ex := r.data.Exec(ctx)
	oldPath := n.Path
	if oldPath == "" {
		oldPath = "/"
	}
	affected, err := ex.Node().Update().
		Where(node.IDEQ(n.ID)).
		SetParentID(newParentID).
		SetPath(newPath).
		AddDepth(depthDelta).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("data: relocate node: %w", err)
	}
	if affected == 0 {
		return biz.ErrNotFound
	}
	descendants, err := ex.Node().Query().
		Where(node.PathHasPrefix(oldPath), node.IDNEQ(n.ID)).
		Select(node.FieldID, node.FieldPath, node.FieldDepth).
		All(ctx)
	if err != nil {
		return fmt.Errorf("data: load subtree: %w", err)
	}
	for _, child := range descendants {
		rest := strings.TrimPrefix(child.Path, oldPath)
		if err := ex.Node().UpdateOneID(child.ID).
			SetPath(newPath + rest).
			AddDepth(depthDelta).
			Exec(ctx); err != nil {
			return fmt.Errorf("data: rewrite subtree path: %w", err)
		}
	}
	return nil
}

// SetSubtreeStatus flips the lifecycle state of a node and everything below it.
func (r *nodeRepo) SetSubtreeStatus(ctx context.Context, n *biz.Node, status biz.NodeStatus, at *time.Time) (int64, error) {
	update := r.data.Exec(ctx).Node().Update().
		Where(node.PathHasPrefix(n.Path)).
		SetStatus(status)
	switch status {
	case biz.NodeStatusTrashed:
		if at != nil {
			update = update.SetTrashedAt(*at)
		} else {
			update = update.SetTrashedAt(time.Now())
		}
		update = update.SetOriginalParentID(n.ParentID)
	case biz.NodeStatusActive:
		update = update.ClearTrashedAt().SetOriginalParentID(uuid.Nil)
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: set subtree status: %w", err)
	}
	return int64(affected), nil
}

// DeleteNodes removes rows permanently and returns them so the caller can
// release the stored objects.
func (r *nodeRepo) DeleteNodes(ctx context.Context, ids []uuid.UUID) ([]*biz.Node, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ex := r.data.Exec(ctx)
	pos, err := ex.Node().Query().Where(node.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: load nodes for delete: %w", err)
	}
	if _, err := ex.Node().Delete().Where(node.IDIn(ids...)).Exec(ctx); err != nil {
		return nil, fmt.Errorf("data: delete nodes: %w", err)
	}
	return toBizNodes(pos), nil
}

// NextAvailableName returns a sibling name that does not collide yet, keeping
// the extension at the end so "report.pdf" becomes "report (1).pdf".
func (r *nodeRepo) NextAvailableName(ctx context.Context, parentID uuid.UUID, name string) (string, error) {
	base, ext := splitName(name)
	prefix := strings.ToLower(base)
	pos, err := r.data.Exec(ctx).Node().Query().
		Where(node.ParentIDEQ(parentID), node.StatusEQ(biz.NodeStatusActive), node.NameLowerContains(prefix)).
		Select(node.FieldNameLower).
		All(ctx)
	if err != nil {
		return "", fmt.Errorf("data: list sibling names: %w", err)
	}
	taken := make(map[string]struct{}, len(pos))
	for _, po := range pos {
		taken[po.NameLower] = struct{}{}
	}
	for i := 1; i < 100000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, ok := taken[strings.ToLower(candidate)]; !ok {
			return candidate, nil
		}
	}
	return "", biz.ErrNameConflict
}

// SubtreeStats aggregates a subtree. The subtree is loaded once and folded in
// memory, which keeps the result independent of the storage dialect.
func (r *nodeRepo) SubtreeStats(ctx context.Context, id uuid.UUID, includeTrashed bool) (*biz.NodeStats, error) {
	root, err := r.FindNodeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	pos, err := r.subtreeRows(ctx, root, includeTrashed)
	if err != nil {
		return nil, err
	}
	stats := &biz.NodeStats{NodeID: id, CategorySizes: map[string]int64{}}
	for _, po := range pos {
		if po.Status == biz.NodeStatusTrashed {
			stats.TrashedCount++
			stats.TrashedSize += po.Size
			continue
		}
		if po.Kind == biz.NodeKindFile {
			stats.FileCount++
			stats.TotalSize += po.Size
			if po.Size > stats.LargestFileSize {
				stats.LargestFileSize = po.Size
				stats.LargestFileName = po.Name
			}
			stats.CategorySizes[mimeFamily(po.MimeType)] += po.Size
		} else {
			stats.FolderCount++
		}
	}
	return stats, nil
}

func (r *nodeRepo) CategorySizes(ctx context.Context, id uuid.UUID, includeTrashed bool) (map[string]int64, error) {
	root, err := r.FindNodeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	pos, err := r.subtreeRows(ctx, root, includeTrashed)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, po := range pos {
		if po.Kind != biz.NodeKindFile {
			continue
		}
		if po.Status == biz.NodeStatusTrashed && !includeTrashed {
			continue
		}
		out[mimeFamily(po.MimeType)] += po.Size
	}
	return out, nil
}

func (r *nodeRepo) subtreeRows(ctx context.Context, root *biz.Node, includeTrashed bool) ([]*ent.Node, error) {
	query := r.data.Exec(ctx).Node().Query().Where(node.PathHasPrefix(root.Path))
	if !includeTrashed {
		query = query.Where(node.StatusEQ(biz.NodeStatusActive))
	}
	pos, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: load subtree: %w", err)
	}
	return pos, nil
}

func toBizNodes(pos []*ent.Node) []*biz.Node {
	out := make([]*biz.Node, 0, len(pos))
	for _, po := range pos {
		out = append(out, toBizNode(po))
	}
	return out
}

// splitName separates a file name from its extension. A dotfile such as
// ".gitignore" has no extension: treating the whole name as one would leave an
// empty base once the conflict suffix is appended.
func splitName(name string) (string, string) {
	ext := path.Ext(name)
	if ext == "" || len(ext) > 16 || ext == name {
		return name, ""
	}
	return strings.TrimSuffix(name, ext), ext
}

// mimeFamily buckets a content type for the storage breakdown.
func mimeFamily(mimeType string) string {
	if mimeType == "" {
		return "other"
	}
	family := mimeType
	if idx := strings.Index(mimeType, "/"); idx > 0 {
		family = mimeType[:idx]
	}
	switch family {
	case "image", "video", "audio", "text", "application", "font", "model":
		return family
	default:
		return "other"
	}
}

// aclRepo persists node access control entries.
type aclRepo struct {
	data *Data
}

// NewAclRepo creates a new AclRepo instance.
func NewAclRepo(d *Data) biz.AclRepo { return &aclRepo{data: d} }

func toBizAcl(po *ent.NodeAcl) *biz.AclEntry {
	if po == nil {
		return nil
	}
	return &biz.AclEntry{
		ID:          po.ID,
		NodeID:      po.NodeID,
		SubjectType: po.SubjectType,
		SubjectID:   po.SubjectID,
		Effect:      po.Effect,
		Permissions: po.Permissions,
		Inherit:     po.Inherit,
		CreatedBy:   po.CreatedBy,
		CreatedAt:   po.CreatedAt,
	}
}

func (r *aclRepo) ListAcl(ctx context.Context, nodeID uuid.UUID) ([]*biz.AclEntry, error) {
	pos, err := r.data.Exec(ctx).NodeAcl().Query().
		Where(nodeacl.NodeIDEQ(nodeID)).
		Order(nodeacl.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list acl: %w", err)
	}
	return toBizAcls(pos), nil
}

func (r *aclRepo) ListAclForNodes(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]*biz.AclEntry, error) {
	out := make(map[uuid.UUID][]*biz.AclEntry, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	pos, err := r.data.Exec(ctx).NodeAcl().Query().
		Where(nodeacl.NodeIDIn(ids...)).
		Order(nodeacl.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list acl for nodes: %w", err)
	}
	for _, po := range pos {
		out[po.NodeID] = append(out[po.NodeID], toBizAcl(po))
	}
	return out, nil
}

// ReplaceAcl swaps the complete entry set of a node. Entries that repeat the
// same subject are collapsed, last one wins, so a malformed request cannot
// leave two contradictory rows behind.
func (r *aclRepo) ReplaceAcl(ctx context.Context, nodeID uuid.UUID, entries []*biz.AclEntry) error {
	ex := r.data.Exec(ctx)
	if _, err := ex.NodeAcl().Delete().Where(nodeacl.NodeIDEQ(nodeID)).Exec(ctx); err != nil {
		return fmt.Errorf("data: clear acl: %w", err)
	}
	if len(entries) == 0 {
		return nil
	}
	unique := make(map[string]*biz.AclEntry, len(entries))
	order := make([]string, 0, len(entries))
	for _, e := range entries {
		key := fmt.Sprintf("%d:%s", e.SubjectType, e.SubjectID)
		if _, ok := unique[key]; !ok {
			order = append(order, key)
		}
		unique[key] = e
	}
	sort.Strings(order)
	for _, key := range order {
		e := unique[key]
		builder := ex.NodeAcl().Create().
			SetNodeID(nodeID).
			SetSubjectType(e.SubjectType).
			SetSubjectID(e.SubjectID).
			SetEffect(e.Effect).
			SetPermissions(e.Permissions).
			SetInherit(e.Inherit).
			SetCreatedBy(e.CreatedBy)
		if e.ID != uuid.Nil {
			builder = builder.SetID(e.ID)
		}
		if _, err := builder.Save(ctx); err != nil {
			return fmt.Errorf("data: create acl entry: %w", err)
		}
	}
	return nil
}

func (r *aclRepo) DeleteAclForNodes(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := r.data.Exec(ctx).NodeAcl().Delete().Where(nodeacl.NodeIDIn(ids...)).Exec(ctx); err != nil {
		return fmt.Errorf("data: delete acl: %w", err)
	}
	return nil
}

func toBizAcls(pos []*ent.NodeAcl) []*biz.AclEntry {
	out := make([]*biz.AclEntry, 0, len(pos))
	for _, po := range pos {
		out = append(out, toBizAcl(po))
	}
	return out
}
