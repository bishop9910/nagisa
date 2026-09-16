package biz

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"nagisa/internal/pkg/crypt"

	"github.com/google/uuid"
)

// fakeUserRepo answers the lookups the built-in account tests need. The
// embedded interface panics on anything else, which is what a test wants.
type fakeUserRepo struct {
	UserRepo

	byID       map[uuid.UUID]*User
	byUsername map[string]*User
}

func newFakeUserRepo(users ...*User) *fakeUserRepo {
	repo := &fakeUserRepo{byID: map[uuid.UUID]*User{}, byUsername: map[string]*User{}}
	for _, user := range users {
		repo.byID[user.ID] = user
		repo.byUsername[user.Username] = user
	}
	return repo
}

func (r *fakeUserRepo) FindUserByID(_ context.Context, id uuid.UUID) (*User, error) {
	if user, ok := r.byID[id]; ok {
		return user, nil
	}
	return nil, ErrNotFound
}

func (r *fakeUserRepo) FindUserByUsername(_ context.Context, username string) (*User, error) {
	if user, ok := r.byUsername[username]; ok {
		return user, nil
	}
	return nil, ErrNotFound
}

func (r *fakeUserRepo) ListUsers(context.Context, ...ListOption) ([]*User, error) {
	out := make([]*User, 0, len(r.byID))
	for _, user := range r.byID {
		out = append(out, user)
	}
	return out, nil
}

func (r *fakeUserRepo) CountUsers(context.Context, ...ListOption) (int64, error) {
	return int64(len(r.byID)), nil
}

func (r *fakeUserRepo) UpdateUser(_ context.Context, user *User) (*User, error) {
	r.byID[user.ID] = user
	r.byUsername[user.Username] = user
	return user, nil
}

func (r *fakeUserRepo) DeleteUser(_ context.Context, id uuid.UUID) error {
	delete(r.byID, id)
	return nil
}

func (r *fakeUserRepo) RevokeUserSessions(context.Context, uuid.UUID) error { return nil }

// passthroughTx runs the function without a real transaction.
type passthroughTx struct{ TxManager }

func (passthroughTx) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func builtinUsecase(repo UserRepo) *UserUsecase {
	return NewUserUsecase(repo, passthroughTx{}, UserUsecaseOptions{
		AdminUsername: "admin",
		GuestUsername: "guest",
	})
}

func TestBuiltinAccountsCannotBeManaged(t *testing.T) {
	adminID, guestID, userID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	admin := &User{ID: adminID, Username: "admin", Role: RoleAdmin, Rank: RankAdmin, Permissions: PermAll, Status: UserStatusActive}
	guest := &User{ID: guestID, Username: "guest", Role: RoleGuest, Rank: RankGuest, Permissions: DefaultPermsFor(RoleGuest), Status: UserStatusActive}
	member := &User{ID: userID, Username: "member", Role: RoleUser, Rank: RankUser, Permissions: DefaultPermsFor(RoleUser), Status: UserStatusActive}
	uc := builtinUsecase(newFakeUserRepo(admin, guest, member))

	// The administrator outranks the guest, so only the built-in rule stops it.
	actor := account(adminID, RoleAdmin, RankAdmin, PermAll)
	ctx := NewContext(context.Background(), actor)

	readOnly := PermView | PermDownload
	grant := PermAll
	if _, err := uc.UpdateUser(ctx, &UpdateUserInput{ID: guestID, Permissions: &grant}); err != ErrPermissionDenied {
		t.Errorf("granting permissions on the built-in guest = %v, want ErrPermissionDenied", err)
	}
	if _, err := uc.SetPermissions(ctx, guestID, readOnly); err != ErrPermissionDenied {
		t.Errorf("setting the guest permission set = %v, want ErrPermissionDenied", err)
	}
	role := RoleManager
	if _, err := uc.UpdateUser(ctx, &UpdateUserInput{ID: guestID, Role: &role}); err != ErrPermissionDenied {
		t.Errorf("changing the guest role = %v, want ErrPermissionDenied", err)
	}
	nickname := "anonymous"
	if _, err := uc.UpdateUser(ctx, &UpdateUserInput{ID: guestID, Nickname: &nickname}); err != ErrPermissionDenied {
		t.Errorf("editing the guest profile = %v, want ErrPermissionDenied", err)
	}
	if err := uc.DeleteUser(ctx, guestID); err != ErrPermissionDenied {
		t.Errorf("deleting the built-in guest = %v, want ErrPermissionDenied", err)
	}
	if err := uc.DeleteUser(ctx, adminID); err != ErrPermissionDenied {
		t.Errorf("deleting the built-in administrator = %v, want ErrPermissionDenied", err)
	}

	// An ordinary account stays manageable, otherwise the lock would be a
	// blanket ban rather than a built-in account rule.
	updated, err := uc.UpdateUser(ctx, &UpdateUserInput{ID: userID, Permissions: &readOnly})
	if err != nil {
		t.Fatalf("updating an ordinary account: %v", err)
	}
	if updated.Permissions != readOnly {
		t.Errorf("ordinary account permissions = %v, want %v", updated.Permissions, readOnly)
	}
}

func TestBuiltinAccountsAreMarkedOnRead(t *testing.T) {
	adminID, guestID, userID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	admin := &User{ID: adminID, Username: "admin", Role: RoleAdmin, Rank: RankAdmin, Permissions: PermAll, Status: UserStatusActive}
	guest := &User{ID: guestID, Username: "Guest", Role: RoleGuest, Rank: RankGuest, Permissions: DefaultPermsFor(RoleGuest), Status: UserStatusActive}
	member := &User{ID: userID, Username: "member", Role: RoleUser, Rank: RankUser, Permissions: DefaultPermsFor(RoleUser), Status: UserStatusActive}
	uc := builtinUsecase(newFakeUserRepo(admin, guest, member))
	ctx := NewContext(context.Background(), account(adminID, RoleAdmin, RankAdmin, PermAll))

	// The configured name is matched without regard to case.
	found, err := uc.GetUser(ctx, guestID)
	if err != nil {
		t.Fatalf("get guest: %v", err)
	}
	if !found.Locked {
		t.Error("the built-in guest was not marked as locked")
	}
	me, err := uc.GetUser(ctx, adminID)
	if err != nil {
		t.Fatalf("get admin: %v", err)
	}
	if !me.Locked {
		t.Error("the built-in administrator was not marked as locked")
	}
	users, _, err := uc.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	locked := map[string]bool{}
	for _, user := range users {
		locked[user.Username] = user.Locked
	}
	if !locked["admin"] || !locked["Guest"] {
		t.Errorf("listed accounts lost the lock flag: %v", locked)
	}
	if locked["member"] {
		t.Error("an ordinary account was marked as locked")
	}
}

func TestGuestLoginIssuesAReadOnlySession(t *testing.T) {
	guestID := uuid.Must(uuid.NewV7())
	guest := &User{
		ID: guestID, Username: "guest", Nickname: "访客", Role: RoleGuest, Rank: RankGuest,
		Permissions: DefaultPermsFor(RoleGuest), Status: UserStatusActive,
	}
	repo := newFakeUserRepo(guest)
	uc := NewAuthUsecase(repo, passthroughTx{}, nil, nil, crypt.NewIssuer("test-secret", "test"), nil,
		AuthUsecaseOptions{GuestUsername: "guest", GuestAutoLogin: true, AccessTokenTTL: time.Hour})

	ctx := context.Background()
	pair, user, err := uc.GuestLogin(ctx)
	if err != nil {
		t.Fatalf("guest login: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("guest login returned no access token")
	}
	if pair.RefreshToken != "" {
		t.Error("guest login returned a refresh token: an anonymous visitor must not leave a session behind")
	}
	if !user.Locked {
		t.Error("the guest account handed to the client is not marked as locked")
	}

	// The token has to authenticate as the guest and carry nothing but read.
	caller, err := uc.Authenticate(ctx, pair.AccessToken)
	if err != nil {
		t.Fatalf("authenticate guest token: %v", err)
	}
	if caller.UserID != guestID {
		t.Errorf("caller = %s, want the guest account %s", caller.UserID, guestID)
	}
	if caller.Perms != PermView|PermDownload {
		t.Errorf("guest permissions = %v, want view and download", caller.Perms)
	}
	if err := caller.Require(PermUpload); err != ErrPermissionDenied {
		t.Errorf("a guest session could upload: %v", err)
	}
}

func TestGuestLoginRespectsTheDeploymentSwitch(t *testing.T) {
	guest := &User{
		ID: uuid.Must(uuid.NewV7()), Username: "guest", Role: RoleGuest, Rank: RankGuest,
		Permissions: DefaultPermsFor(RoleGuest), Status: UserStatusActive,
	}
	issuer := crypt.NewIssuer("test-secret", "test")
	box, err := crypt.LoadOrCreateKeyBox("", filepath.Join(t.TempDir(), "password_key.pem"))
	if err != nil {
		t.Fatalf("key box: %v", err)
	}

	disabled := NewAuthUsecase(newFakeUserRepo(guest), passthroughTx{}, nil, nil, issuer, box, AuthUsecaseOptions{})
	if _, _, err := disabled.GuestLogin(context.Background()); err != ErrUnsupported {
		t.Errorf("guest login while disabled = %v, want ErrUnsupported", err)
	}
	if disabled.Config().GuestLoginEnabled {
		t.Error("the public config advertises guest login while it is switched off")
	}

	enabled := NewAuthUsecase(newFakeUserRepo(guest), passthroughTx{}, nil, nil, issuer, box,
		AuthUsecaseOptions{GuestUsername: "guest", GuestAutoLogin: true})
	if !enabled.Config().GuestLoginEnabled {
		t.Error("the public config hides guest login while it is switched on")
	}

	// A disabled guest account must not be able to hand out a session.
	guest.Status = UserStatusDisabled
	if _, _, err := enabled.GuestLogin(context.Background()); err != ErrAccountDisabled {
		t.Errorf("guest login on a disabled account = %v, want ErrAccountDisabled", err)
	}
}
