package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"
	"nagisa/internal/pkg/crypt"

	"github.com/google/uuid"
)

// GetSystemInfo 是前端启动引导（以及管理后台的「系统信息」页）唯一的信息来源，
// AuthConfig 里少填一个字段，前端就只能显示零值——曾经漏过 guest_login_enabled，
// 于是部署明明开着免登录访客，页面上却是「已关闭」。
func TestGetSystemInfoCarriesEveryAuthField(t *testing.T) {
	auth, sys := testAuthUsecase(t), testSystemUsecase(nil)
	svc := NewSystemService(sys, auth)

	info, err := svc.GetSystemInfo(context.Background(), &v1.GetSystemInfoRequest{})
	if err != nil {
		t.Fatalf("get system info: %v", err)
	}
	authInfo := info.GetAuth()
	if authInfo == nil {
		t.Fatal("the reply carries no auth config")
	}

	if !authInfo.GetGuestLoginEnabled() {
		t.Error("guest_login_enabled is not reported although the deployment enables it")
	}
	if authInfo.GetPlainPasswordAllowed() {
		t.Error("plain_password_allowed is true although the deployment forbids it")
	}
	if got := authInfo.GetMinPasswordLength(); got != 12 {
		t.Errorf("min_password_length = %d, want 12", got)
	}
	if got := authInfo.GetAccessTokenTtlSeconds(); got != 7200 {
		t.Errorf("access_token_ttl_seconds = %d, want 7200", got)
	}
	if got := authInfo.GetRefreshTokenTtlSeconds(); got != 30*24*3600 {
		t.Errorf("refresh_token_ttl_seconds = %d, want %d", got, 30*24*3600)
	}
	if authInfo.GetPasswordEncoding() != crypt.PasswordEncoding {
		t.Errorf("password_encoding = %q, want %q", authInfo.GetPasswordEncoding(), crypt.PasswordEncoding)
	}
	if authInfo.GetPasswordKeyId() == "" {
		t.Error("password_key_id is empty, so a key rotation could not be detected")
	}
	if !strings.Contains(authInfo.GetPasswordPublicKey(), "BEGIN PUBLIC KEY") {
		t.Error("password_public_key is not a PEM public key")
	}
	if info.GetServerTime() == nil || authInfo.GetServerTime() == nil {
		t.Error("the reply carries no server time")
	}
}

// system.version 是只读键：它没有写入路径，值必须是进程自身的构建版本，
// 不能把数据库里可能躺着的旧值回显出来，也不能空着。
func TestListSystemSettingsDerivesTheReadOnlyVersion(t *testing.T) {
	sys := testSystemUsecase(map[string]string{
		"upload.max_versions": "7",
		"system.version":      "stale-value-from-the-database",
	})
	svc := NewSystemService(sys, testAuthUsecase(t))

	reply, err := svc.ListSystemSettings(settingsContext(), &v1.ListSystemSettingsRequest{})
	if err != nil {
		t.Fatalf("list system settings: %v", err)
	}

	values := map[string]string{}
	for _, setting := range reply.GetSettings() {
		values[setting.GetKey()] = setting.GetValue()
		if setting.GetKey() == "system.version" && setting.GetWritable() {
			t.Error("system.version is advertised as writable")
		}
	}
	if got := values["system.version"]; got != "v9.9.9" {
		t.Errorf("system.version = %q, want the build version %q", got, "v9.9.9")
	}
	if got := values["upload.max_versions"]; got != "7" {
		t.Errorf("upload.max_versions = %q, want the stored value %q", got, "7")
	}
}

// fakeSettingRepo answers the runtime tunable reads the tests need.
type fakeSettingRepo struct {
	biz.SettingRepo

	values map[string]string
}

func (r *fakeSettingRepo) ListSettings(context.Context) (map[string]string, error) {
	return r.values, nil
}

func (r *fakeSettingRepo) PutSettings(context.Context, map[string]string) error { return nil }

func testSystemUsecase(stored map[string]string) *biz.SystemUsecase {
	if stored == nil {
		stored = map[string]string{}
	}
	return biz.NewSystemUsecase(nil, nil, nil, nil, nil, nil, &fakeSettingRepo{values: stored}, nil, nil, biz.SystemUsecaseOptions{
		Info: biz.SystemInfo{Name: "Nagisa", Version: "v9.9.9", APIVersion: "v1"},
	})
}

func testAuthUsecase(t *testing.T) *biz.AuthUsecase {
	t.Helper()
	box, err := crypt.LoadOrCreateKeyBox("", filepath.Join(t.TempDir(), "password_key.pem"))
	if err != nil {
		t.Fatalf("key box: %v", err)
	}
	return biz.NewAuthUsecase(nil, nil, nil, nil, crypt.NewIssuer("test-secret", "test"), box, biz.AuthUsecaseOptions{
		AccessTokenTTL:     2 * time.Hour,
		RefreshTokenTTL:    30 * 24 * time.Hour,
		PlainPasswordAllow: false,
		MinPasswordLength:  12,
		GuestUsername:      "guest",
		GuestAutoLogin:     true,
	})
}

// settingsContext is a caller that may read the runtime tunables.
func settingsContext() context.Context {
	return biz.NewContext(context.Background(), &biz.Caller{
		UserID: uuid.Must(uuid.NewV7()),
		Role:   biz.RoleAdmin,
		Rank:   biz.RankAdmin,
		Perms:  biz.PermAll,
		Status: biz.UserStatusActive,
	})
}
