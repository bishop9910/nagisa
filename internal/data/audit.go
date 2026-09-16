package data

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/data/ent"
	"nagisa/internal/data/ent/auditlog"
	"nagisa/internal/data/ent/predicate"

	"github.com/go-kratos/aip-go/ents"
	"github.com/google/uuid"
)

const (
	// defaultAuditPageSize is used when the caller does not set a limit.
	defaultAuditPageSize = 100
	// maxAuditDayRows bounds the day bucket scan described in AuditByDay.
	maxAuditDayRows = 1_000_000
)

// auditRepo persists the append-only audit trail.
type auditRepo struct {
	data *Data
}

// NewAuditRepo returns an audit repository.
func NewAuditRepo(d *Data) biz.AuditRepo { return &auditRepo{data: d} }

// CreateAuditLogs appends a batch of entries.
func (r *auditRepo) CreateAuditLogs(ctx context.Context, logs []*biz.AuditLog) error {
	if len(logs) == 0 {
		return nil
	}
	client := r.data.Exec(ctx).AuditLog()
	builders := make([]*ent.AuditLogCreate, 0, len(logs))
	for _, entry := range logs {
		builders = append(builders, newAuditLogCreate(client, entry))
	}
	if _, err := client.CreateBulk(builders...).Save(ctx); err != nil {
		return fmt.Errorf("data: create audit logs: %w", err)
	}
	return nil
}

// ListAuditLogs returns a page of audit entries.
func (r *auditRepo) ListAuditLogs(ctx context.Context, opts ...biz.ListOption) ([]*biz.AuditLog, error) {
	o := biz.ResolveListOptions(defaultAuditPageSize, opts...)
	rows, err := auditQuery(r.data.Exec(ctx).AuditLog().Query(), o).
		Offset(o.Offset).
		Limit(o.Limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list audit logs: %w", err)
	}
	return toBizAuditLogs(rows), nil
}

// CountAuditLogs counts the entries matching the options, ignoring pagination.
func (r *auditRepo) CountAuditLogs(ctx context.Context, opts ...biz.ListOption) (int64, error) {
	o := biz.ResolveListOptions(defaultAuditPageSize, opts...)
	n, err := auditQuery(r.data.Exec(ctx).AuditLog().Query(), o).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: count audit logs: %w", err)
	}
	return int64(n), nil
}

// FindAuditLogByID loads one audit entry.
func (r *auditRepo) FindAuditLogByID(ctx context.Context, id uuid.UUID) (*biz.AuditLog, error) {
	po, err := r.data.Exec(ctx).AuditLog().Query().Where(auditlog.ID(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find audit log: %w", err)
	}
	return toBizAuditLog(po), nil
}

// PurgeAuditLogs deletes the entries recorded before the given instant.
func (r *auditRepo) PurgeAuditLogs(ctx context.Context, before time.Time) (int64, error) {
	deleted, err := r.data.Exec(ctx).AuditLog().Delete().
		Where(auditlog.CreatedAtLT(before)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: purge audit logs: %w", err)
	}
	return int64(deleted), nil
}

// AuditByAction counts the entries of each action inside the window.
func (r *auditRepo) AuditByAction(ctx context.Context, from, to time.Time, prefix string, limit int) ([]biz.AuditCount, error) {
	var rows []struct {
		Action string `json:"action,omitempty"`
		Count  int64  `json:"count,omitempty"`
	}
	err := r.data.Exec(ctx).AuditLog().Query().
		Where(auditWindow(from, to, prefix)...).
		GroupBy(auditlog.FieldAction).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("data: audit by action: %w", err)
	}
	counts := make([]biz.AuditCount, 0, len(rows))
	for _, row := range rows {
		counts = append(counts, biz.AuditCount{
			Key:   row.Action,
			Label: biz.ActionLabel(row.Action),
			Count: row.Count,
		})
	}
	return rankAuditCounts(counts, limit), nil
}

// AuditByActor counts the entries of each actor inside the window.
func (r *auditRepo) AuditByActor(ctx context.Context, from, to time.Time, prefix string, limit int) ([]biz.AuditCount, error) {
	var rows []struct {
		ActorID   uuid.UUID `json:"actor_id,omitempty"`
		ActorName string    `json:"actor_name,omitempty"`
		Count     int64     `json:"count,omitempty"`
	}
	err := r.data.Exec(ctx).AuditLog().Query().
		Where(auditWindow(from, to, prefix)...).
		GroupBy(auditlog.FieldActorID, auditlog.FieldActorName).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("data: audit by actor: %w", err)
	}
	counts := make([]biz.AuditCount, 0, len(rows))
	for _, row := range rows {
		label := row.ActorName
		if label == "" {
			label = row.ActorID.String()
		}
		counts = append(counts, biz.AuditCount{
			Key:   row.ActorID.String(),
			Label: label,
			Count: row.Count,
		})
	}
	return rankAuditCounts(counts, limit), nil
}

// AuditByDay counts the entries of each calendar day inside the window. ent
// cannot group by a date expression, so the rows of the (already bounded)
// window are bucketed in Go and maxAuditDayRows caps the scan.
func (r *auditRepo) AuditByDay(ctx context.Context, from, to time.Time, prefix string) (map[string]int64, error) {
	rows, err := r.data.Exec(ctx).AuditLog().Query().
		Where(auditWindow(from, to, prefix)...).
		Order(auditlog.ByCreatedAt()).
		Limit(maxAuditDayRows).
		Select(auditlog.FieldCreatedAt).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: audit by day: %w", err)
	}
	byDay := make(map[string]int64, 7)
	for _, row := range rows {
		byDay[row.CreatedAt.UTC().Format(time.DateOnly)]++
	}
	return byDay, nil
}

// CountAuditFailures counts the failed entries inside the window.
func (r *auditRepo) CountAuditFailures(ctx context.Context, from, to time.Time, prefix string) (int64, error) {
	preds := append(auditWindow(from, to, prefix), auditlog.SuccessEQ(false))
	n, err := r.data.Exec(ctx).AuditLog().Query().Where(preds...).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: count audit failures: %w", err)
	}
	return int64(n), nil
}

// auditQuery applies the standard AIP filter and ordering to a list query. The
// creation time is appended last so offset pagination stays stable when the
// caller orders by a non unique column.
func auditQuery(q *ent.AuditLogQuery, o biz.ListOptions) *ent.AuditLogQuery {
	return q.
		Where(predicate.AuditLog(ents.ApplyFilter(o.Filter))).
		Order(auditlog.OrderOption(ents.ApplyOrderBy(o.OrderBy)), auditlog.ByCreatedAt())
}

// auditWindow builds the predicates of an aggregation window. An empty prefix
// constrains nothing.
func auditWindow(from, to time.Time, prefix string) []predicate.AuditLog {
	preds := make([]predicate.AuditLog, 0, 3)
	if !from.IsZero() {
		preds = append(preds, auditlog.CreatedAtGTE(from))
	}
	if !to.IsZero() {
		preds = append(preds, auditlog.CreatedAtLTE(to))
	}
	if prefix != "" {
		preds = append(preds, auditlog.ActionHasPrefix(prefix))
	}
	return preds
}

// rankAuditCounts orders the aggregated rows by count and keeps the head.
func rankAuditCounts(counts []biz.AuditCount, limit int) []biz.AuditCount {
	slices.SortStableFunc(counts, func(a, b biz.AuditCount) int {
		if a.Count != b.Count {
			return cmp.Compare(b.Count, a.Count)
		}
		return strings.Compare(a.Key, b.Key)
	})
	if limit > 0 && len(counts) > limit {
		counts = counts[:limit]
	}
	return counts
}

// newAuditLogCreate builds the insert statement of one trail entry.
func newAuditLogCreate(c *ent.AuditLogClient, entry *biz.AuditLog) *ent.AuditLogCreate {
	builder := c.Create().
		SetActorID(entry.ActorID).
		SetActorName(entry.ActorName).
		SetAction(entry.Action).
		SetTargetType(entry.TargetType).
		SetTargetID(entry.TargetID).
		SetTargetName(entry.TargetName).
		SetSuccess(entry.Success).
		SetErrorReason(entry.ErrorReason).
		SetIP(entry.IP).
		SetUserAgent(entry.UserAgent).
		SetRequestID(entry.RequestID)
	if entry.ID != uuid.Nil {
		builder.SetID(entry.ID)
	}
	if entry.Detail != nil {
		builder.SetDetail(entry.Detail)
	}
	if !entry.CreatedAt.IsZero() {
		builder.SetCreatedAt(entry.CreatedAt)
	}
	return builder
}

// toBizAuditLog converts a stored entry into its domain shape.
func toBizAuditLog(po *ent.AuditLog) *biz.AuditLog {
	if po == nil {
		return nil
	}
	return &biz.AuditLog{
		ID:          po.ID,
		ActorID:     po.ActorID,
		ActorName:   po.ActorName,
		Action:      po.Action,
		TargetType:  po.TargetType,
		TargetID:    po.TargetID,
		TargetName:  po.TargetName,
		Success:     po.Success,
		ErrorReason: po.ErrorReason,
		Detail:      po.Detail,
		IP:          po.IP,
		UserAgent:   po.UserAgent,
		RequestID:   po.RequestID,
		CreatedAt:   po.CreatedAt,
	}
}

// toBizAuditLogs converts a list of stored entries.
func toBizAuditLogs(rows []*ent.AuditLog) []*biz.AuditLog {
	out := make([]*biz.AuditLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBizAuditLog(row))
	}
	return out
}
