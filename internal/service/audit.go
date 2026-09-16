package service

import (
	"context"
	"time"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"

	"go.einride.tech/aip/filtering"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AuditService implements the audit trail API. Reading requires
// PERMISSION_AUDIT_READ; a caller without it only ever sees its own entries.
type AuditService struct {
	v1.UnimplementedAuditServiceServer

	uc *biz.AuditUsecase
}

// NewAuditService new an audit service.
func NewAuditService(uc *biz.AuditUsecase) *AuditService {
	return &AuditService{uc: uc}
}

// auditPageSize caps one page of audit entries.
const auditPageSize = 500

// auditDeclarations describes the filterable fields of an audit entry.
func auditDeclarations() (*filtering.Declarations, error) {
	return declarations(
		filtering.DeclareIdent("actor_id", filtering.TypeString),
		filtering.DeclareIdent("actor_name", filtering.TypeString),
		filtering.DeclareIdent("action", filtering.TypeString),
		filtering.DeclareIdent("target_type", filtering.TypeString),
		filtering.DeclareIdent("target_id", filtering.TypeString),
		filtering.DeclareIdent("success", filtering.TypeBool),
		filtering.DeclareIdent("ip", filtering.TypeString),
		filtering.DeclareIdent("created_at", filtering.TypeTimestamp),
	)
}

// auditOrderPaths lists the orderable fields of an audit entry.
var auditOrderPaths = []string{"created_at", "action", "actor_name"}

// ListAuditLogs returns a page of audit entries.
func (s *AuditService) ListAuditLogs(ctx context.Context, req *v1.ListAuditLogsRequest) (*v1.AuditLogSet, error) {
	decl, err := auditDeclarations()
	if err != nil {
		return nil, err
	}
	// The AIP grammar cannot express boolean literals, so `success=true` and
	// `success=false` are rewritten into the bare-identifier form (see
	// normalizeBoolLiterals).
	req.Filter = normalizeBoolLiterals(req.Filter, "success")
	opts, size, token, err := parseList(req, decl, auditPageSize, auditOrderPaths...)
	if err != nil {
		return nil, err
	}
	logs, total, err := s.uc.List(ctx, opts...)
	if err != nil {
		return nil, err
	}
	caller := biz.CallerFromContext(ctx)
	if !caller.Has(biz.PermAuditRead) {
		// Without the audit permission the caller only sees its own entries.
		// The page is narrowed here rather than by widening the query, and the
		// total shrinks with it so the reply never counts foreign entries.
		owned := make([]*biz.AuditLog, 0, len(logs))
		for _, entry := range logs {
			if entry.ActorID == caller.UserID {
				owned = append(owned, entry)
			}
		}
		logs, total = owned, int64(len(owned))
	}
	set := &v1.AuditLogSet{
		Logs:      make([]*v1.AuditLog, 0, len(logs)),
		TotalSize: total,
	}
	for _, entry := range logs {
		set.Logs = append(set.Logs, convertAuditLog(entry))
	}
	set.NextPageToken = nextPageToken(req, token, len(logs), int(size))
	return set, nil
}

// GetAuditLog returns a single audit entry.
func (s *AuditService) GetAuditLog(ctx context.Context, req *v1.GetAuditLogRequest) (*v1.AuditLog, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	entry, err := s.uc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return convertAuditLog(entry), nil
}

// GetAuditSummary aggregates the trail by action, actor and day.
func (s *AuditService) GetAuditSummary(ctx context.Context, req *v1.GetAuditSummaryRequest) (*v1.AuditSummary, error) {
	var from, to time.Time
	if req.GetFrom() != nil {
		from = req.GetFrom().AsTime()
	}
	if req.GetTo() != nil {
		to = req.GetTo().AsTime()
	}
	summary, err := s.uc.Summary(ctx, from, to, req.GetActionPrefix())
	if err != nil {
		return nil, err
	}
	out := &v1.AuditSummary{
		From:         timestamppb.New(summary.From),
		To:           timestamppb.New(summary.To),
		Total:        summary.Total,
		FailureCount: summary.FailureCount,
		ByAction:     make([]*v1.AuditActionCount, 0, len(summary.ByAction)),
		ByActor:      make([]*v1.AuditActorCount, 0, len(summary.ByActor)),
		ByDay:        summary.ByDay,
	}
	for _, count := range summary.ByAction {
		out.ByAction = append(out.ByAction, &v1.AuditActionCount{
			Action:        count.Key,
			ActionDisplay: biz.ActionLabel(count.Key),
			Count:         count.Count,
		})
	}
	for _, count := range summary.ByActor {
		out.ByActor = append(out.ByActor, &v1.AuditActorCount{
			ActorId:   count.Key,
			ActorName: count.Label,
			Count:     count.Count,
		})
	}
	return out, nil
}
