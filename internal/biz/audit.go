package biz

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuditLog is one recorded action of the append-only audit trail.
type AuditLog struct {
	ID          uuid.UUID
	ActorID     uuid.UUID
	ActorName   string
	Action      string
	TargetType  string
	TargetID    string
	TargetName  string
	Success     bool
	ErrorReason string
	Detail      map[string]string
	IP          string
	UserAgent   string
	RequestID   string
	CreatedAt   time.Time
}

// AuditCount is one row of an audit aggregation.
type AuditCount struct {
	Key   string
	Label string
	Count int64
}

// AuditSummary aggregates the audit trail over a window.
type AuditSummary struct {
	From         time.Time
	To           time.Time
	Total        int64
	FailureCount int64
	ByAction     []AuditCount
	ByActor      []AuditCount
	ByDay        map[string]int64
}

// AuditRepo persists audit entries.
type AuditRepo interface {
	CreateAuditLogs(context.Context, []*AuditLog) error
	ListAuditLogs(context.Context, ...ListOption) ([]*AuditLog, error)
	CountAuditLogs(context.Context, ...ListOption) (int64, error)
	FindAuditLogByID(context.Context, uuid.UUID) (*AuditLog, error)
	PurgeAuditLogs(context.Context, time.Time) (int64, error)
	AuditByAction(context.Context, time.Time, time.Time, string, int) ([]AuditCount, error)
	AuditByActor(context.Context, time.Time, time.Time, string, int) ([]AuditCount, error)
	AuditByDay(context.Context, time.Time, time.Time, string) (map[string]int64, error)
	CountAuditFailures(context.Context, time.Time, time.Time, string) (int64, error)
}

// AuditUsecase exposes the audit trail.
type AuditUsecase struct {
	repo AuditRepo
}

// NewAuditUsecase returns an audit usecase.
func NewAuditUsecase(repo AuditRepo) *AuditUsecase {
	return &AuditUsecase{repo: repo}
}

// Repo exposes the repository so the transport middleware can append entries
// without going through an authorisation check.
func (uc *AuditUsecase) Repo() AuditRepo { return uc.repo }

// Record appends entries to the trail.
func (uc *AuditUsecase) Record(ctx context.Context, entries ...*AuditLog) error {
	if len(entries) == 0 {
		return nil
	}
	for _, e := range entries {
		if e.ID == uuid.Nil {
			e.ID = NewID()
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = time.Now()
		}
	}
	return uc.repo.CreateAuditLogs(ctx, entries)
}

// List returns a page of audit entries. The filter is expected to restrict the
// result to the caller's own entries when it lacks PERMISSION_AUDIT_READ; the
// service layer enforces that by injecting an actor filter.
func (uc *AuditUsecase) List(ctx context.Context, opts ...ListOption) ([]*AuditLog, int64, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, 0, ErrUnauthenticated
	}
	logs, err := uc.repo.ListAuditLogs(ctx, opts...)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.repo.CountAuditLogs(ctx, opts...)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// Get returns one audit entry.
func (uc *AuditUsecase) Get(ctx context.Context, id uuid.UUID) (*AuditLog, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	entry, err := uc.repo.FindAuditLogByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !caller.Has(PermAuditRead) && entry.ActorID != caller.UserID {
		return nil, ErrPermissionDenied
	}
	return entry, nil
}

// Summary aggregates the trail over a window.
func (uc *AuditUsecase) Summary(ctx context.Context, from, to time.Time, prefix string) (*AuditSummary, error) {
	caller := CallerFromContext(ctx)
	if err := caller.Require(PermAuditRead); err != nil {
		return nil, err
	}
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		from = to.Add(-7 * 24 * time.Hour)
	}
	byAction, err := uc.repo.AuditByAction(ctx, from, to, prefix, 50)
	if err != nil {
		return nil, err
	}
	byActor, err := uc.repo.AuditByActor(ctx, from, to, prefix, 20)
	if err != nil {
		return nil, err
	}
	byDay, err := uc.repo.AuditByDay(ctx, from, to, prefix)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, c := range byAction {
		total += c.Count
	}
	failures, err := uc.repo.CountAuditFailures(ctx, from, to, prefix)
	if err != nil {
		return nil, err
	}
	return &AuditSummary{
		From: from, To: to, Total: total, FailureCount: failures,
		ByAction: byAction, ByActor: byActor, ByDay: byDay,
	}, nil
}

// auditActionLabels maps machine action names onto Chinese descriptions.
var auditActionLabels = map[string]string{
	"auth.login":             "登录",
	"auth.logout":            "退出登录",
	"auth.refresh":           "刷新令牌",
	"auth.password_change":   "修改密码",
	"user.create":            "创建账号",
	"user.update":            "修改账号",
	"user.delete":            "删除账号",
	"user.permissions_set":   "设置账号权限",
	"user.password_reset":    "重置账号密码",
	"node.create_folder":     "新建文件夹",
	"node.update":            "修改文件或文件夹",
	"node.move":              "移动",
	"node.copy":              "复制",
	"node.delete":            "移入回收站",
	"node.restore":           "从回收站还原",
	"node.purge":             "彻底删除",
	"node.trash_empty":       "清空回收站",
	"node.acl_set":           "设置访问权限",
	"node.unlock":            "解锁加密文件夹",
	"node.version_restore":   "还原历史版本",
	"node.version_delete":    "删除历史版本",
	"file.upload_initiate":   "发起上传",
	"file.upload_complete":   "完成上传",
	"file.upload_abort":      "取消上传",
	"file.upload_inline":     "上传小文件",
	"file.download":          "下载",
	"file.archive":           "打包下载",
	"share.create":           "创建分享",
	"share.update":           "修改分享",
	"share.delete":           "取消分享",
	"share.access":           "访问分享链接",
	"share.download":         "通过分享下载",
	"system.maintenance":     "执行维护任务",
	"system.settings_update": "修改系统设置",
	"audit.list":             "查看审计日志",
}

// ActionLabel returns the Chinese description of an action.
func ActionLabel(action string) string {
	if label, ok := auditActionLabels[action]; ok {
		return label
	}
	return action
}
