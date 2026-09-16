package data

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"nagisa/internal/biz"
	"nagisa/internal/conf"
	"nagisa/internal/data/ent"
	"nagisa/internal/data/ent/setting"
	"nagisa/internal/data/ent/user"
	"nagisa/internal/pkg/crypt"

	"github.com/google/uuid"
)

// Seeder provisions the accounts and settings a fresh deployment needs. It
// runs once during startup, before the servers accept traffic, and is
// idempotent: an existing account is left untouched so a restart never resets
// a password.
type Seeder struct {
	data    *Data
	hasher  *crypt.Hasher
	auth    *conf.Auth
	storage *conf.Storage
}

// NewSeeder prepares and runs the bootstrap step.
func NewSeeder(d *Data, hasher *crypt.Hasher, c *conf.Bootstrap) (*Seeder, error) {
	s := &Seeder{data: d, hasher: hasher, auth: c.GetAuth(), storage: c.GetStorage()}
	if err := s.Run(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

// Run creates the built-in administrator and the read-only guest account when
// they are missing, and seeds the default settings.
func (s *Seeder) Run(ctx context.Context) error {
	adminName := strings.TrimSpace(s.auth.GetAdminUsername())
	if adminName == "" {
		adminName = "admin"
	}
	guestName := strings.TrimSpace(s.auth.GetGuestUsername())
	if guestName == "" {
		guestName = "guest"
	}

	err := s.data.WithTx(ctx, func(ctx context.Context) error {
		if err := s.ensureAccount(ctx, accountSpec{
			Username:     adminName,
			Nickname:     "超级管理员",
			Role:         biz.RoleAdmin,
			Rank:         biz.RankAdmin,
			Permissions:  biz.PermAll,
			Password:     s.auth.GetAdminPassword(),
			LogGenerated: true,
			Remark:       "系统内置管理员，不可被其他账号管理",
		}); err != nil {
			return err
		}
		if err := s.ensureAccount(ctx, accountSpec{
			Username:    guestName,
			Nickname:    "访客",
			Role:        biz.RoleGuest,
			Rank:        biz.RankGuest,
			Permissions: biz.DefaultPermsFor(biz.RoleGuest),
			QuotaBytes:  s.storage.GetGuestQuotaBytes(),
			Remark:      "系统内置访客账号，默认只读，权限不可修改；无密码，经 AuthService.GuestLogin 进入",
		}); err != nil {
			return err
		}
		// The API refuses to change either built-in account, so repairing them
		// here keeps the guarantee true even after a direct database edit.
		if err := s.restoreAuthority(ctx, adminName, biz.RoleAdmin, biz.RankAdmin, biz.PermAll); err != nil {
			return err
		}
		return s.restoreAuthority(ctx, guestName, biz.RoleGuest, biz.RankGuest, biz.DefaultPermsFor(biz.RoleGuest))
	})
	if err != nil {
		return err
	}
	return s.ensureSettings(ctx)
}

// accountSpec describes a bootstrap account.
type accountSpec struct {
	Username    string
	Nickname    string
	Role        biz.Role
	Rank        int32
	Permissions biz.PermMask
	// Password is the initial password. Empty means a random one is
	// generated.
	Password string
	// LogGenerated announces a generated password once, which is only useful
	// for the administrator: the built-in guest is entered through
	// AuthService.GuestLogin, which takes no password at all, so printing one
	// would only hint at a credential that cannot be used.
	LogGenerated bool
	QuotaBytes   int64
	Remark       string
}

// ensureAccount creates one account when it does not exist yet.
func (s *Seeder) ensureAccount(ctx context.Context, spec accountSpec) error {
	ex, err := s.data.Exec(ctx).User().Query().
		Where(user.UsernameEQ(spec.Username)).
		Exist(ctx)
	if err != nil {
		return fmt.Errorf("data: probe account %q: %w", spec.Username, err)
	}
	if ex {
		return nil
	}
	password := spec.Password
	generated := false
	if password == "" {
		raw, err := crypt.RandomToken(12)
		if err != nil {
			return err
		}
		password = raw
		generated = true
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("data: hash bootstrap password: %w", err)
	}
	if _, err := s.data.Exec(ctx).User().Create().
		SetUsername(spec.Username).
		SetNickname(spec.Nickname).
		SetPasswordHash(hash).
		SetRole(spec.Role).
		SetRank(spec.Rank).
		SetPermissions(spec.Permissions).
		SetStatus(biz.UserStatusActive).
		SetQuotaBytes(spec.QuotaBytes).
		SetRemark(spec.Remark).
		Save(ctx); err != nil {
		return fmt.Errorf("data: create bootstrap account %q: %w", spec.Username, err)
	}
	switch {
	case generated && spec.LogGenerated:
		// The generated password is printed exactly once. Configure
		// auth.admin_password to control it.
		slog.Warn("bootstrap account created with a generated password, change it after the first login",
			"username", spec.Username,
			"password", password,
		)
	case generated:
		// The account stores a random hash nobody ever learns, which is what
		// makes a password login for it impossible.
		slog.Info("bootstrap account created without a usable password",
			"username", spec.Username)
	default:
		slog.Info("bootstrap account created", "username", spec.Username)
	}
	return nil
}

// restoreAuthority puts a built-in account back on the role, rank, permission
// set and status the deployment fixes for it. Nothing else about the account is
// touched, and a missing account is not recreated here: ensureAccount already
// ran.
func (s *Seeder) restoreAuthority(ctx context.Context, username string, role biz.Role, rank int32, perms biz.PermMask) error {
	account, err := s.data.Exec(ctx).User().Query().Where(user.UsernameEQ(username)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("data: probe built-in account %q: %w", username, err)
	}
	if account.Role == role && account.Rank == rank && account.Permissions == perms && account.Status == biz.UserStatusActive {
		return nil
	}
	slog.Warn("restored the fixed authority of a built-in account",
		"username", username, "role", biz.RoleName(role), "rank", rank)
	if _, err := s.data.Exec(ctx).User().UpdateOneID(account.ID).
		SetRole(role).
		SetRank(rank).
		SetPermissions(perms).
		SetStatus(biz.UserStatusActive).
		Save(ctx); err != nil {
		return fmt.Errorf("data: restore built-in account %q: %w", username, err)
	}
	return nil
}

// ensureSettings writes the runtime tunables that ship with sensible defaults.
func (s *Seeder) ensureSettings(ctx context.Context) error {
	defaults := map[string]string{
		"guest.default_permissions":   fmt.Sprint(int64(biz.DefaultPermsFor(biz.RoleGuest))),
		"storage.default_quota_bytes": "0",
		"upload.keep_versions":        "true",
		"upload.max_versions":         "20",
		"share.allow_public":          "true",
		"trash.retention_days":        "30",
		"system.maintenance_interval": "1h",
	}
	for key, value := range defaults {
		exists, err := s.data.Exec(ctx).Setting().Query().Where(setting.KeyEQ(key)).Exist(ctx)
		if err != nil {
			return fmt.Errorf("data: probe setting %q: %w", key, err)
		}
		if exists {
			continue
		}
		if _, err := s.data.Exec(ctx).Setting().Create().
			SetID(uuid.Must(uuid.NewV7())).
			SetKey(key).
			SetValue(value).
			Save(ctx); err != nil {
			return fmt.Errorf("data: seed setting %q: %w", key, err)
		}
	}
	return nil
}
