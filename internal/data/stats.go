package data

import (
	"context"
	"fmt"
	"strings"

	"nagisa/internal/biz"
	"nagisa/internal/data/ent"
	"nagisa/internal/data/ent/node"
	"nagisa/internal/data/ent/nodeversion"
	"nagisa/internal/data/ent/predicate"
	"nagisa/internal/data/ent/share"
	"nagisa/internal/data/ent/upload"
	"nagisa/internal/data/ent/user"

	"entgo.io/ent/dialect/sql"
)

// statsMimeFamilies are the mime families of the storage breakdown.
var statsMimeFamilies = map[string]bool{
	"image":       true,
	"video":       true,
	"audio":       true,
	"text":        true,
	"application": true,
}

// statsCounter is the counting capability every generated query builder shares.
type statsCounter interface {
	Count(context.Context) (int, error)
}

// statsRepo aggregates the storage counters.
type statsRepo struct {
	data *Data
}

// NewStatsRepo returns a statistics repository.
func NewStatsRepo(d *Data) biz.StatsRepo { return &statsRepo{data: d} }

// StorageStats aggregates the storage counters across the schema.
func (r *statsRepo) StorageStats(ctx context.Context, topOwners int) (*biz.StorageStats, error) {
	e := r.data.Exec(ctx)
	stats := &biz.StorageStats{SizeByCategory: map[string]int64{}}
	var err error

	if stats.TotalBytes, err = statsSum(ctx, e, node.Status(biz.NodeStatusActive)); err != nil {
		return nil, err
	}
	if stats.TotalFiles, err = statsCount(ctx, "total files", e.Node().Query().
		Where(node.Status(biz.NodeStatusActive), node.Kind(biz.NodeKindFile))); err != nil {
		return nil, err
	}
	if stats.TotalFolders, err = statsCount(ctx, "total folders", e.Node().Query().
		Where(node.Status(biz.NodeStatusActive), node.Kind(biz.NodeKindFolder))); err != nil {
		return nil, err
	}
	if stats.TrashedBytes, err = statsSum(ctx, e, node.Status(biz.NodeStatusTrashed)); err != nil {
		return nil, err
	}
	if stats.TrashedNodes, err = statsCount(ctx, "trashed nodes", e.Node().Query().
		Where(node.Status(biz.NodeStatusTrashed))); err != nil {
		return nil, err
	}
	if stats.TotalUsers, err = statsCount(ctx, "total users", e.User().Query()); err != nil {
		return nil, err
	}
	if stats.ActiveUsers, err = statsCount(ctx, "active users", e.User().Query().
		Where(user.Status(biz.UserStatusActive))); err != nil {
		return nil, err
	}
	if stats.DisabledUsers, err = statsCount(ctx, "disabled users", e.User().Query().
		Where(user.Status(biz.UserStatusDisabled))); err != nil {
		return nil, err
	}
	if stats.UploadsInProgress, err = statsCount(ctx, "uploads in progress", e.Upload().Query().
		Where(upload.StatusIn(biz.UploadStatusPending, biz.UploadStatusInProgress))); err != nil {
		return nil, err
	}
	if stats.ActiveShares, err = statsCount(ctx, "active shares", e.Share().Query().
		Where(share.Status(biz.ShareStatusActive))); err != nil {
		return nil, err
	}
	if stats.VersionsBytes, err = statsVersionBytes(ctx, e); err != nil {
		return nil, err
	}
	if stats.SizeByCategory, err = statsSizeByCategory(ctx, e); err != nil {
		return nil, err
	}
	if stats.TopOwners, err = statsTopOwners(ctx, e, topOwners); err != nil {
		return nil, err
	}
	return stats, nil
}

// BackendUsedBytes sums the size of every live object.
func (r *statsRepo) BackendUsedBytes(ctx context.Context) (int64, error) {
	objects := r.data.Objects()
	if !objects.Enabled() {
		return 0, nil
	}
	infos, err := objects.ListPrefix(ctx, "")
	if err != nil {
		return 0, err
	}
	var total int64
	for _, info := range infos {
		total += info.Size
	}
	return total, nil
}

// statsSum returns the summed size of the nodes matching the predicates.
func statsSum(ctx context.Context, e exec, preds ...predicate.Node) (int64, error) {
	var rows []struct {
		Total int64 `json:"total,omitempty"`
	}
	err := e.Node().Query().
		Where(preds...).
		Aggregate(ent.As(ent.Sum(node.FieldSize), "total")).
		Scan(ctx, &rows)
	if err != nil {
		return 0, fmt.Errorf("data: sum node sizes: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Total, nil
}

// statsVersionBytes returns the summed size of every retained revision.
func statsVersionBytes(ctx context.Context, e exec) (int64, error) {
	var rows []struct {
		Total int64 `json:"total,omitempty"`
	}
	err := e.NodeVersion().Query().
		Aggregate(ent.As(ent.Sum(nodeversion.FieldSize), "total")).
		Scan(ctx, &rows)
	if err != nil {
		return 0, fmt.Errorf("data: sum version sizes: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Total, nil
}

// statsSizeByCategory sums the active file bytes per mime family.
func statsSizeByCategory(ctx context.Context, e exec) (map[string]int64, error) {
	var rows []struct {
		MimeType string `json:"mime_type,omitempty"`
		Size     int64  `json:"size,omitempty"`
	}
	err := e.Node().Query().
		Where(node.Status(biz.NodeStatusActive), node.Kind(biz.NodeKindFile)).
		GroupBy(node.FieldMimeType).
		Aggregate(ent.As(ent.Sum(node.FieldSize), "size")).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("data: sum node sizes by category: %w", err)
	}
	byCategory := make(map[string]int64, len(statsMimeFamilies))
	for _, row := range rows {
		byCategory[statsMimeFamily(row.MimeType)] += row.Size
	}
	return byCategory, nil
}

// statsTopOwners returns the accounts with the largest recorded usage.
func statsTopOwners(ctx context.Context, e exec, topOwners int) ([]biz.OwnerUsage, error) {
	if topOwners <= 0 {
		return nil, nil
	}
	rows, err := e.User().Query().
		Order(user.ByUsedBytes(sql.OrderDesc()), user.ByID()).
		Limit(topOwners).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list top owners: %w", err)
	}
	owners := make([]biz.OwnerUsage, 0, len(rows))
	for _, row := range rows {
		owners = append(owners, biz.OwnerUsage{
			OwnerID:     row.ID,
			OwnerName:   statsOwnerName(row),
			UsedBytes:   row.UsedBytes,
			FileCount:   row.FileCount,
			FolderCount: row.FolderCount,
			QuotaBytes:  row.QuotaBytes,
		})
	}
	return owners, nil
}

// statsOwnerName returns the label an account is displayed under.
func statsOwnerName(u *ent.User) string {
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.Username
}

// statsMimeFamily maps a mime type onto the family of the storage breakdown.
func statsMimeFamily(mimeType string) string {
	family, _, _ := strings.Cut(mimeType, "/")
	family = strings.ToLower(strings.TrimSpace(family))
	if statsMimeFamilies[family] {
		return family
	}
	return "other"
}

// statsCount labels a counter query and widens its result.
func statsCount(ctx context.Context, label string, query statsCounter) (int64, error) {
	n, err := query.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: storage stats %s: %w", label, err)
	}
	return int64(n), nil
}
