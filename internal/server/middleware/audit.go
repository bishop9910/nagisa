package middleware

import (
	"context"
	"sync"
	"time"

	"nagisa/internal/biz"

	"github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/log"
	kratosmw "github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
)

// auditActions maps a proto operation onto the audit action it records. Both
// transports report the fully qualified method name, so one table covers HTTP
// and gRPC.
var auditActions = map[string]struct{ Action, Target string }{
	"/netdisk.v1.AuthService/Login":                  {"auth.login", "session"},
	"/netdisk.v1.AuthService/RefreshToken":           {"auth.refresh", "session"},
	"/netdisk.v1.AuthService/Logout":                 {"auth.logout", "session"},
	"/netdisk.v1.AuthService/ChangePassword":         {"auth.password_change", "user"},
	"/netdisk.v1.UserService/CreateUser":             {"user.create", "user"},
	"/netdisk.v1.UserService/UpdateUser":             {"user.update", "user"},
	"/netdisk.v1.UserService/DeleteUser":             {"user.delete", "user"},
	"/netdisk.v1.UserService/SetUserPermissions":     {"user.permissions_set", "user"},
	"/netdisk.v1.UserService/ResetUserPassword":      {"user.password_reset", "user"},
	"/netdisk.v1.NodeService/CreateFolder":           {"node.create_folder", "node"},
	"/netdisk.v1.NodeService/UpdateNode":             {"node.update", "node"},
	"/netdisk.v1.NodeService/MoveNodes":              {"node.move", "node"},
	"/netdisk.v1.NodeService/CopyNodes":              {"node.copy", "node"},
	"/netdisk.v1.NodeService/DeleteNodes":            {"node.delete", "node"},
	"/netdisk.v1.NodeService/RestoreNodes":           {"node.restore", "node"},
	"/netdisk.v1.NodeService/PurgeNodes":             {"node.purge", "node"},
	"/netdisk.v1.NodeService/EmptyTrash":             {"node.trash_empty", "node"},
	"/netdisk.v1.NodeService/SetNodeAcl":             {"node.acl_set", "node"},
	"/netdisk.v1.NodeService/UnlockNode":             {"node.unlock", "node"},
	"/netdisk.v1.NodeService/RestoreNodeVersion":     {"node.version_restore", "node"},
	"/netdisk.v1.NodeService/DeleteNodeVersion":      {"node.version_delete", "node"},
	"/netdisk.v1.FileService/InitiateUpload":         {"file.upload_initiate", "upload"},
	"/netdisk.v1.FileService/CompleteUpload":         {"file.upload_complete", "upload"},
	"/netdisk.v1.FileService/AbortUpload":            {"file.upload_abort", "upload"},
	"/netdisk.v1.FileService/ConfirmUploadPart":      {"file.upload_part", "upload"},
	"/netdisk.v1.FileService/UploadChunk":            {"file.upload_part", "upload"},
	"/netdisk.v1.FileService/UploadSmallFile":        {"file.upload_inline", "node"},
	"/netdisk.v1.FileService/GetDownloadUrl":         {"file.download", "node"},
	"/netdisk.v1.FileService/GetArchiveUrl":          {"file.archive", "node"},
	"/netdisk.v1.ShareService/CreateShare":           {"share.create", "share"},
	"/netdisk.v1.ShareService/UpdateShare":           {"share.update", "share"},
	"/netdisk.v1.ShareService/DeleteShare":           {"share.delete", "share"},
	"/netdisk.v1.ShareService/AccessShare":           {"share.access", "share"},
	"/netdisk.v1.ShareService/GetShareDownloadUrl":   {"share.download", "share"},
	"/netdisk.v1.SystemService/RunMaintenance":       {"system.maintenance", "system"},
	"/netdisk.v1.SystemService/UpdateSystemSettings": {"system.settings_update", "system"},
}

// Auditor records what the API did. Entries are buffered and written by a
// single background worker so a slow audit write can never fail a request, and
// so a burst of calls costs one batch insert instead of one insert each.
type Auditor struct {
	repo    biz.AuditRepo
	tx      biz.TxManager
	entries chan *biz.AuditLog
	done    chan struct{}
	once    sync.Once
}

// auditorBuffer is the number of entries kept in memory before writers start
// dropping them, which is a deliberate trade: losing an audit line under a
// pathological burst beats blocking the API.
const auditorBuffer = 4096

// NewAuditor starts the background writer.
func NewAuditor(repo biz.AuditRepo, tx biz.TxManager) *Auditor {
	a := &Auditor{
		repo:    repo,
		tx:      tx,
		entries: make(chan *biz.AuditLog, auditorBuffer),
		done:    make(chan struct{}),
	}
	go a.run()
	return a
}

// Close flushes the buffer and stops the worker.
func (a *Auditor) Close() {
	a.once.Do(func() {
		close(a.entries)
		<-a.done
	})
}

func (a *Auditor) run() {
	defer close(a.done)
	batch := make([]*biz.AuditLog, 0, 64)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		// The insert runs inside the same serialised write path as the
		// request that produced it, so a burst of audit writes can never
		// collide with a live transaction.
		err := a.tx.WithTx(context.Background(), func(ctx context.Context) error {
			return a.repo.CreateAuditLogs(ctx, batch)
		})
		if err != nil {
			log.Error("audit: write failed", "err", err, "count", len(batch))
		}
		batch = batch[:0]
	}
	for {
		entry, ok := <-a.entries
		if !ok {
			flush()
			return
		}
		batch = append(batch, entry)
		// Coalesce whatever else is already queued into the same insert.
	drain:
		for len(batch) < 64 {
			select {
			case more, ok := <-a.entries:
				if !ok {
					break drain
				}
				batch = append(batch, more)
			default:
				break drain
			}
		}
		flush()
	}
}

// Record queues one entry, dropping it when the buffer is saturated.
func (a *Auditor) Record(entry *biz.AuditLog) {
	select {
	case a.entries <- entry:
	default:
		log.Warn("audit: buffer full, entry dropped", "action", entry.Action)
	}
}

// Middleware records one entry per state changing call. Reads are skipped so
// the trail stays a record of changes rather than of traffic.
func (a *Auditor) Middleware() kratosmw.Middleware {
	return func(next kratosmw.Handler) kratosmw.Handler {
		return func(ctx context.Context, req any) (any, error) {
			out, err := next(ctx, req)
			if a == nil {
				return out, err
			}
			action, target, ok := a.describe(ctx)
			if !ok {
				return out, err
			}
			caller := biz.CallerFromContext(ctx)
			entry := &biz.AuditLog{
				Action:     action,
				TargetType: target,
				TargetID:   targetIDOf(ctx, req),
				Success:    err == nil,
				IP:         caller.IP,
				UserAgent:  caller.UserAgent,
				RequestID:  caller.RequestID,
				CreatedAt:  time.Now(),
			}
			if caller.IsAuthenticated() {
				entry.ActorID = caller.UserID
				entry.ActorName = caller.Username
			}
			if err != nil {
				entry.ErrorReason = errors.Reason(err)
				if entry.ErrorReason == "" {
					entry.ErrorReason = err.Error()
				}
			}
			a.Record(entry)
			return out, err
		}
	}
}

// describe resolves the route of the current request into an action.
func (a *Auditor) describe(ctx context.Context) (string, string, bool) {
	carrier, ok := transport.FromServerContext(ctx)
	if !ok {
		return "", "", false
	}
	key := carrier.Operation()
	meta, ok := auditActions[key]
	if !ok {
		return "", "", false
	}
	return meta.Action, meta.Target, true
}

// targetIDOf extracts the identifier of the affected resource from the request
// message, falling back to the route variables.
func targetIDOf(ctx context.Context, req any) string {
	for _, getter := range []func(any) string{
		func(v any) string {
			if g, ok := v.(interface{ GetId() string }); ok {
				return g.GetId()
			}
			return ""
		},
		func(v any) string {
			if g, ok := v.(interface{ GetNodeId() string }); ok {
				return g.GetNodeId()
			}
			return ""
		},
		func(v any) string {
			if g, ok := v.(interface{ GetUploadId() string }); ok {
				return g.GetUploadId()
			}
			return ""
		},
	} {
		if id := getter(req); id != "" {
			return id
		}
	}
	return ""
}
