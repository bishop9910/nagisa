package service

import (
	"context"
	"time"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// startedAt is the process start time, used to report uptime.
var startedAt = time.Now()

// SystemService implements the deployment level API. GetSystemInfo and
// HealthCheck are public; every other method requires an administrator.
type SystemService struct {
	v1.UnimplementedSystemServiceServer

	uc   *biz.SystemUsecase
	auth *biz.AuthUsecase
}

// NewSystemService new a system service.
func NewSystemService(uc *biz.SystemUsecase, auth *biz.AuthUsecase) *SystemService {
	return &SystemService{uc: uc, auth: auth}
}

// GetSystemInfo returns the capabilities, limits and public auth parameters of
// this deployment. This endpoint is public.
func (s *SystemService) GetSystemInfo(ctx context.Context, req *v1.GetSystemInfoRequest) (*v1.SystemInfo, error) {
	info := s.uc.Info()
	auth := s.auth.Config()
	now := timestamppb.Now()
	out := &v1.SystemInfo{
		Name:                    info.Name,
		Version:                 info.Version,
		ApiVersion:              info.APIVersion,
		Features:                info.Features,
		MaxUploadSize:           info.MaxUploadSize,
		DefaultChunkSize:        info.DefaultChunkSize,
		MinChunkSize:            info.MinChunkSize,
		MaxInlineSize:           info.MaxInlineSize,
		UploadSessionTtlSeconds: int32(info.UploadSessionTTL.Seconds()),
		SignedUrlTtlSeconds:     int32(info.SignedURLTTL.Seconds()),
		SignedUrlMaxTtlSeconds:  int32(info.SignedURLMaxTTL.Seconds()),
		UploadModes:             make([]v1.UploadMode, 0, len(info.UploadModes)),
		StorageBackend:          info.StorageBackend,
		DatabaseBackend:         info.DatabaseBackend,
		DefaultVisibility:       convertVisibility(info.DefaultVisibility),
		RegistrationEnabled:     info.RegistrationOpen,
		Auth: &v1.AuthConfig{
			PasswordKeyId:          auth.PasswordKeyID,
			PasswordEncoding:       auth.PasswordEncoding,
			PasswordPublicKey:      auth.PasswordPublicKey,
			AccessTokenTtlSeconds:  int32(auth.AccessTokenTTL.Seconds()),
			RefreshTokenTtlSeconds: int32(auth.RefreshTokenTTL.Seconds()),
			PlainPasswordAllowed:   auth.PlainPasswordAllowed,
			MinPasswordLength:      int32(auth.MinPasswordLength),
			ServerTime:             now,
		},
		ServerTime:    now,
		PublicBaseUrl: info.PublicBaseURL,
	}
	for _, mode := range info.UploadModes {
		out.UploadModes = append(out.UploadModes, convertUploadMode(mode))
	}
	return out, nil
}

// HealthCheck reports whether the process can reach its dependencies. This
// endpoint is public.
func (s *SystemService) HealthCheck(ctx context.Context, req *v1.HealthCheckRequest) (*v1.HealthStatus, error) {
	status, checks, err := s.uc.Health(ctx, req.GetDeep())
	if err != nil {
		return nil, err
	}
	return &v1.HealthStatus{
		Status:        status,
		Checks:        checks,
		UptimeSeconds: int64(time.Since(startedAt).Seconds()),
		ServerTime:    timestamppb.Now(),
	}, nil
}

// GetStorageStats aggregates storage usage across accounts.
func (s *SystemService) GetStorageStats(ctx context.Context, req *v1.GetStorageStatsRequest) (*v1.StorageStats, error) {
	stats, err := s.uc.Storage(ctx, int(req.GetTopOwners()))
	if err != nil {
		return nil, err
	}
	out := &v1.StorageStats{
		TotalBytes:        stats.TotalBytes,
		TotalFiles:        stats.TotalFiles,
		TotalFolders:      stats.TotalFolders,
		TotalUsers:        stats.TotalUsers,
		ActiveUsers:       stats.ActiveUsers,
		DisabledUsers:     stats.DisabledUsers,
		TrashedBytes:      stats.TrashedBytes,
		TrashedNodes:      stats.TrashedNodes,
		UploadsInProgress: stats.UploadsInProgress,
		ActiveShares:      stats.ActiveShares,
		VersionsBytes:     stats.VersionsBytes,
		SizeByCategory:    stats.SizeByCategory,
		TopOwners:         make([]*v1.OwnerUsage, 0, len(stats.TopOwners)),
		BackendUsedBytes:  stats.BackendUsedBytes,
	}
	for _, owner := range stats.TopOwners {
		out.TopOwners = append(out.TopOwners, &v1.OwnerUsage{
			OwnerId:     uuidOrEmpty(owner.OwnerID),
			OwnerName:   owner.OwnerName,
			UsedBytes:   owner.UsedBytes,
			FileCount:   owner.FileCount,
			FolderCount: owner.FolderCount,
			QuotaBytes:  owner.QuotaBytes,
		})
	}
	return out, nil
}

// RunMaintenance triggers the housekeeping jobs.
func (s *SystemService) RunMaintenance(ctx context.Context, req *v1.RunMaintenanceRequest) (*v1.MaintenanceReport, error) {
	report, err := s.uc.Maintenance(ctx, req.GetTasks(), req.GetDryRun(), req.GetTrashRetentionDays())
	if err != nil {
		return nil, err
	}
	return &v1.MaintenanceReport{
		DryRun:         report.DryRun,
		Tasks:          report.Tasks,
		ExpiredUploads: report.ExpiredUploads,
		DeletedObjects: report.DeletedObjects,
		OrphanObjects:  report.OrphanObjects,
		ExpiredShares:  report.ExpiredShares,
		RecountedUsers: report.RecountedUsers,
		PurgedNodes:    report.PurgedNodes,
		Warnings:       report.Warnings,
		DurationMs:     report.Duration.Milliseconds(),
		FinishedAt:     timestamppb.New(report.FinishedAt),
	}, nil
}

// ListSystemSettings returns the runtime tunables that may be changed without a
// restart.
func (s *SystemService) ListSystemSettings(ctx context.Context, req *v1.ListSystemSettingsRequest) (*v1.SystemSettingSet, error) {
	settings, err := s.uc.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.SystemSettingSet{Settings: convertSettings(settings)}, nil
}

// UpdateSystemSettings writes runtime tunables.
func (s *SystemService) UpdateSystemSettings(ctx context.Context, req *v1.UpdateSystemSettingsRequest) (*v1.SystemSettingSet, error) {
	in := make([]biz.Setting, 0, len(req.GetSettings()))
	for _, setting := range req.GetSettings() {
		in = append(in, biz.Setting{
			Key:         setting.GetKey(),
			Value:       setting.GetValue(),
			Type:        setting.GetType(),
			Description: setting.GetDescription(),
			Writable:    setting.GetWritable(),
		})
	}
	settings, err := s.uc.UpdateSettings(ctx, in)
	if err != nil {
		return nil, err
	}
	return &v1.SystemSettingSet{Settings: convertSettings(settings)}, nil
}

// convertSettings renders runtime tunables.
func convertSettings(in []biz.Setting) []*v1.SystemSetting {
	out := make([]*v1.SystemSetting, 0, len(in))
	for _, setting := range in {
		out = append(out, &v1.SystemSetting{
			Key:         setting.Key,
			Value:       setting.Value,
			Type:        setting.Type,
			Description: setting.Description,
			Writable:    setting.Writable,
		})
	}
	return out
}
