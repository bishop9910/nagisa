package biz

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// OwnerUsage is one row of the per-account usage breakdown.
type OwnerUsage struct {
	OwnerID     uuid.UUID
	OwnerName   string
	UsedBytes   int64
	FileCount   int64
	FolderCount int64
	QuotaBytes  int64
}

// StorageStats aggregates storage usage across accounts.
type StorageStats struct {
	TotalBytes        int64
	TotalFiles        int64
	TotalFolders      int64
	TotalUsers        int64
	ActiveUsers       int64
	DisabledUsers     int64
	TrashedBytes      int64
	TrashedNodes      int64
	UploadsInProgress int64
	ActiveShares      int64
	VersionsBytes     int64
	SizeByCategory    map[string]int64
	TopOwners         []OwnerUsage
	BackendUsedBytes  int64
}

// Setting is one runtime tunable.
type Setting struct {
	Key         string
	Value       string
	Type        string
	Description string
	Writable    bool
}

// MaintenanceReport describes what a maintenance run did.
type MaintenanceReport struct {
	DryRun         bool
	Tasks          []string
	ExpiredUploads int64
	DeletedObjects int64
	OrphanObjects  int64
	ExpiredShares  int64
	RecountedUsers int64
	PurgedNodes    int64
	Warnings       []string
	Duration       time.Duration
	FinishedAt     time.Time
}

// StatsRepo aggregates storage counters across the schema.
type StatsRepo interface {
	StorageStats(context.Context, int) (*StorageStats, error)
	BackendUsedBytes(context.Context) (int64, error)
}

// SettingRepo persists runtime tunables.
type SettingRepo interface {
	ListSettings(context.Context) (map[string]string, error)
	PutSettings(context.Context, map[string]string) error
}

// SystemInfo is the static description of a deployment.
type SystemInfo struct {
	Name              string
	Version           string
	APIVersion        string
	Features          []string
	MaxUploadSize     int64
	DefaultChunkSize  int64
	MinChunkSize      int64
	MaxInlineSize     int64
	UploadSessionTTL  time.Duration
	SignedURLTTL      time.Duration
	SignedURLMaxTTL   time.Duration
	UploadModes       []UploadMode
	StorageBackend    string
	DatabaseBackend   string
	DefaultVisibility Visibility
	RegistrationOpen  bool
	PublicBaseURL     string
}

// SystemUsecase exposes deployment information, statistics, settings and the
// maintenance jobs.
type SystemUsecase struct {
	users          UserRepo
	nodes          *NodeUsecase
	nodeRepo       NodeRepo
	files          FileRepo
	shares         ShareRepo
	stats          StatsRepo
	settings       SettingRepo
	store          ObjectStore
	tx             TxManager
	info           SystemInfo
	trashRetention time.Duration
}

// SystemUsecaseOptions bundles the deployment description.
type SystemUsecaseOptions struct {
	Info           SystemInfo
	TrashRetention time.Duration
}

// NewSystemUsecase returns a system usecase.
func NewSystemUsecase(users UserRepo, nodes *NodeUsecase, nodeRepo NodeRepo, files FileRepo, shares ShareRepo, stats StatsRepo, settings SettingRepo, store ObjectStore, tx TxManager, opts SystemUsecaseOptions) *SystemUsecase {
	if opts.TrashRetention <= 0 {
		opts.TrashRetention = 30 * 24 * time.Hour
	}
	return &SystemUsecase{
		users: users, nodes: nodes, nodeRepo: nodeRepo, files: files,
		shares: shares, stats: stats, settings: settings, store: store,
		tx: tx, info: opts.Info, trashRetention: opts.TrashRetention,
	}
}

// Info returns the static deployment description.
func (uc *SystemUsecase) Info() SystemInfo { return uc.info }

// TrashRetention is how long trashed nodes are kept.
func (uc *SystemUsecase) TrashRetention() time.Duration { return uc.trashRetention }

// Health probes the dependencies and reports their reachability.
func (uc *SystemUsecase) Health(ctx context.Context, deep bool) (string, map[string]string, error) {
	checks := map[string]string{"database": "ok", "object_storage": "ok"}
	status := "ok"
	if !deep {
		return status, checks, nil
	}
	if _, err := uc.users.CountByRole(ctx); err != nil {
		checks["database"] = "down: " + err.Error()
		status = "degraded"
	}
	if uc.store != nil {
		if err := uc.store.HealthCheck(ctx); err != nil {
			checks["object_storage"] = "down: " + err.Error()
			status = "degraded"
		}
	}
	return status, checks, nil
}

// Storage returns global storage statistics.
func (uc *SystemUsecase) Storage(ctx context.Context, topOwners int) (*StorageStats, error) {
	caller := CallerFromContext(ctx)
	if err := caller.Require(PermStorageManage); err != nil {
		return nil, err
	}
	if topOwners <= 0 {
		topOwners = 10
	}
	stats, err := uc.stats.StorageStats(ctx, topOwners)
	if err != nil {
		return nil, err
	}
	if uc.store != nil {
		if used, err := uc.stats.BackendUsedBytes(ctx); err == nil {
			stats.BackendUsedBytes = used
		}
	}
	return stats, nil
}

// settingCatalog documents every runtime tunable.
var settingCatalog = []Setting{
	{Key: "guest.default_permissions", Type: "int", Description: "访客账号的默认权限位掩码", Writable: true},
	{Key: "storage.default_quota_bytes", Type: "int", Description: "新建账号的默认配额（字节，0 表示不限）", Writable: true},
	{Key: "upload.max_inline_size", Type: "int", Description: "单请求内联上传的大小上限（字节）", Writable: true},
	{Key: "upload.default_chunk_size", Type: "int", Description: "默认分片大小（字节）", Writable: true},
	{Key: "upload.keep_versions", Type: "bool", Description: "覆盖文件时是否保留历史版本", Writable: true},
	{Key: "upload.max_versions", Type: "int", Description: "每个文件保留的历史版本数量上限", Writable: true},
	{Key: "share.allow_public", Type: "bool", Description: "是否允许匿名访问分享链接", Writable: true},
	{Key: "trash.retention_days", Type: "int", Description: "回收站保留天数", Writable: true},
	{Key: "system.maintenance_interval", Type: "string", Description: "自动维护任务执行间隔，例如 1h", Writable: true},
	{Key: "system.version", Type: "string", Description: "当前构建版本", Writable: false},
}

// Settings returns the runtime tunables together with their current values.
func (uc *SystemUsecase) Settings(ctx context.Context) ([]Setting, error) {
	caller := CallerFromContext(ctx)
	if err := caller.Require(PermStorageManage); err != nil {
		return nil, err
	}
	stored, err := uc.settings.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	// system.version is read only: nothing ever writes it, its value is the build
	// the process is running.
	derived := map[string]string{"system.version": uc.info.Version}
	out := make([]Setting, 0, len(settingCatalog))
	for _, s := range settingCatalog {
		if v, ok := stored[s.Key]; ok {
			s.Value = v
		}
		if v, ok := derived[s.Key]; ok {
			s.Value = v
		}
		out = append(out, s)
	}
	return out, nil
}

// UpdateSettings writes runtime tunables after validating their types.
func (uc *SystemUsecase) UpdateSettings(ctx context.Context, in []Setting) ([]Setting, error) {
	caller := CallerFromContext(ctx)
	if err := caller.Require(PermSystemManage); err != nil {
		return nil, err
	}
	known := map[string]Setting{}
	for _, s := range settingCatalog {
		known[s.Key] = s
	}
	values := map[string]string{}
	for _, s := range in {
		def, ok := known[s.Key]
		if !ok {
			return nil, fmt.Errorf("%w: unknown setting %q", ErrInvalidArgument, s.Key)
		}
		if !def.Writable {
			return nil, fmt.Errorf("%w: setting %q is read only", ErrPermissionDenied, s.Key)
		}
		if err := validateSetting(def.Type, s.Value); err != nil {
			return nil, err
		}
		values[s.Key] = s.Value
	}
	if err := uc.settings.PutSettings(ctx, values); err != nil {
		return nil, err
	}
	return uc.Settings(ctx)
}

func validateSetting(kind, value string) error {
	switch kind {
	case "int":
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("%w: %q is not an integer", ErrInvalidArgument, value)
		}
	case "bool":
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%w: %q is not a boolean", ErrInvalidArgument, value)
		}
	case "json":
		if value == "" {
			return fmt.Errorf("%w: empty json value", ErrInvalidArgument)
		}
	}
	return nil
}

// Maintenance runs the housekeeping jobs. Each task is independent, so one
// failing dependency degrades the report instead of aborting the run.
func (uc *SystemUsecase) Maintenance(ctx context.Context, tasks []string, dryRun bool, retentionDays int32) (*MaintenanceReport, error) {
	caller := CallerFromContext(ctx)
	if err := caller.Require(PermStorageManage); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		tasks = []string{"expire_uploads", "purge_deletions", "expire_shares", "recount_usage"}
	}
	if retentionDays <= 0 {
		retentionDays = int32(uc.trashRetention / (24 * time.Hour))
	}
	started := time.Now()
	report := &MaintenanceReport{DryRun: dryRun, Tasks: tasks, FinishedAt: started}
	for _, task := range tasks {
		switch task {
		case "expire_uploads":
			n, err := uc.expireUploads(ctx, dryRun)
			if err != nil {
				report.Warnings = append(report.Warnings, "expire_uploads: "+err.Error())
				continue
			}
			report.ExpiredUploads = n
		case "purge_deletions":
			n, err := uc.purgeDeletions(ctx, dryRun)
			if err != nil {
				report.Warnings = append(report.Warnings, "purge_deletions: "+err.Error())
				continue
			}
			report.DeletedObjects = n
		case "gc_orphans":
			n, err := uc.gcOrphans(ctx, dryRun)
			if err != nil {
				report.Warnings = append(report.Warnings, "gc_orphans: "+err.Error())
				continue
			}
			report.OrphanObjects = n
		case "expire_shares":
			n, err := uc.expireShares(ctx, dryRun)
			if err != nil {
				report.Warnings = append(report.Warnings, "expire_shares: "+err.Error())
				continue
			}
			report.ExpiredShares = n
		case "recount_usage":
			n, err := uc.recountUsage(ctx, dryRun)
			if err != nil {
				report.Warnings = append(report.Warnings, "recount_usage: "+err.Error())
				continue
			}
			report.RecountedUsers = n
		case "purge_trash":
			n, err := uc.purgeTrash(ctx, dryRun, retentionDays)
			if err != nil {
				report.Warnings = append(report.Warnings, "purge_trash: "+err.Error())
				continue
			}
			report.PurgedNodes = n
		default:
			report.Warnings = append(report.Warnings, "unknown task: "+task)
		}
	}
	report.Duration = time.Since(started)
	report.FinishedAt = time.Now()
	return report, nil
}

// expireUploads retires sessions past their expiry and releases their parts.
func (uc *SystemUsecase) expireUploads(ctx context.Context, dryRun bool) (int64, error) {
	uploads, err := uc.files.ListExpiredUploads(ctx, time.Now(), 500)
	if err != nil {
		return 0, err
	}
	var count int64
	for _, u := range uploads {
		if dryRun {
			count++
			continue
		}
		u.Status = UploadStatusExpired
		u.LastError = "expired by maintenance"
		if _, err := uc.files.UpdateUpload(ctx, u); err != nil {
			return count, err
		}
		if err := uc.files.DeleteUploadParts(ctx, u.ID); err != nil {
			return count, err
		}
		if err := uc.files.EnqueueDeletion(ctx, u.Path, "upload_expired"); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// purgeDeletions drains the object deletion queue.
func (uc *SystemUsecase) purgeDeletions(ctx context.Context, dryRun bool) (int64, error) {
	items, err := uc.files.ListPendingDeletions(ctx, 500)
	if err != nil {
		return 0, err
	}
	var count int64
	for _, item := range items {
		if dryRun {
			count++
			continue
		}
		if err := uc.store.RemovePrefix(ctx, item.Key); err != nil {
			if markErr := uc.files.MarkDeletionAttempt(ctx, item.ID, err.Error()); markErr != nil {
				return count, markErr
			}
			continue
		}
		if err := uc.files.DeletePendingDeletion(ctx, item.ID); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// gcOrphans reclaims staged objects that no longer belong to anything: the
// parts of a session that was aborted or expired without being cleaned up, and
// anything left under the scratch prefix.
//
// It deliberately spares the staging prefix of every session that is still
// open. An in-flight upload's parts are not referenced by any node yet, so a
// naive sweep would delete the bytes of an upload that is about to complete.
func (uc *SystemUsecase) gcOrphans(ctx context.Context, dryRun bool) (int64, error) {
	if uc.store == nil {
		return 0, nil
	}
	live, err := uc.liveUploadPrefixes(ctx)
	if err != nil {
		return 0, err
	}
	var removed int64
	for _, prefix := range []string{"uploads/", "orphan/"} {
		objects, err := uc.store.ListPrefix(ctx, prefix)
		if err != nil {
			return removed, err
		}
		for _, o := range objects {
			if liveUpload(live, o.Key) {
				continue
			}
			if dryRun {
				removed++
				continue
			}
			if err := uc.store.RemoveObject(ctx, o.Key); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

// liveUploadPrefixes returns the staging path of every session that may still
// receive parts.
func (uc *SystemUsecase) liveUploadPrefixes(ctx context.Context) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	now := time.Now()
	for _, status := range []UploadStatus{UploadStatusPending, UploadStatusInProgress} {
		s := status
		uploads, err := uc.files.ListUploads(ctx, nil, &s, ListLimit(10000))
		if err != nil {
			return nil, err
		}
		for _, u := range uploads {
			if u.ExpiresAt.Before(now) {
				continue
			}
			out[u.Path+"/"] = struct{}{}
		}
	}
	return out, nil
}

// liveUpload reports whether an object key belongs to an open session.
func liveUpload(live map[string]struct{}, key string) bool {
	for prefix := range live {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// expireShares marks links whose expiry has passed.
func (uc *SystemUsecase) expireShares(ctx context.Context, dryRun bool) (int64, error) {
	shares, err := uc.shares.ListExpiredShares(ctx, time.Now(), 500)
	if err != nil {
		return 0, err
	}
	if dryRun {
		return int64(len(shares)), nil
	}
	var count int64
	for _, s := range shares {
		s.Status = ShareStatusExpired
		if _, err := uc.shares.UpdateShare(ctx, s); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// recountUsage recomputes the per-account counters from the tree, which
// repairs any drift left by an interrupted operation.
func (uc *SystemUsecase) recountUsage(ctx context.Context, dryRun bool) (int64, error) {
	if dryRun {
		return 0, nil
	}
	return uc.users.RecountAllUsage(ctx)
}

// purgeTrash permanently removes trashed nodes older than the retention window.
func (uc *SystemUsecase) purgeTrash(ctx context.Context, dryRun bool, retentionDays int32) (int64, error) {
	before := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	status := NodeStatusTrashed
	nodes, err := uc.nodeRepo.ListNodes(ctx, NodeQuery{Status: &status, IncludeTrashed: true}, ListLimit(10000))
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
	for _, n := range nodes {
		if n.TrashedAt != nil && n.TrashedAt.Before(before) {
			ids = append(ids, n.ID)
		}
	}
	if len(ids) == 0 || dryRun {
		return int64(len(ids)), nil
	}
	var count int64
	err = uc.tx.WithTx(ctx, func(ctx context.Context) error {
		for _, id := range ids {
			purged, _, err := uc.nodes.PurgeNodes(ctx, []uuid.UUID{id})
			if err != nil {
				return err
			}
			count += int64(len(purged))
		}
		return nil
	})
	return count, err
}
