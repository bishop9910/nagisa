package biz

import (
	"testing"

	v1 "nagisa/api/netdisk/v1"

	"github.com/google/uuid"
)

func account(id uuid.UUID, role Role, rank int32, perms PermMask) *Caller {
	return &Caller{
		UserID:   id,
		Username: "user-" + id.String()[:8],
		Role:     role,
		Rank:     rank,
		Perms:    perms,
		Unlocked: map[uuid.UUID]bool{},
	}
}

func folder(id, owner uuid.UUID, visibility Visibility) *Node {
	return &Node{ID: id, OwnerID: owner, Kind: NodeKindFolder, Status: NodeStatusActive, Visibility: visibility, Path: ChildPath("/", id)}
}

func file(id, owner uuid.UUID, visibility Visibility) *Node {
	return &Node{ID: id, OwnerID: owner, Kind: NodeKindFile, Status: NodeStatusActive, Visibility: visibility}
}

func allow(subjectType SubjectType, subjectID string, perms PermMask, inherit bool) *AclEntry {
	return &AclEntry{SubjectType: subjectType, SubjectID: subjectID, Effect: EffectAllow, Permissions: perms, Inherit: inherit}
}

func deny(subjectType SubjectType, subjectID string, perms PermMask, inherit bool) *AclEntry {
	return &AclEntry{SubjectType: subjectType, SubjectID: subjectID, Effect: EffectDeny, Permissions: perms, Inherit: inherit}
}

// TestPermissionBitsMatchTheAPI pins the storage layout to the proto enum. A
// drift here would silently grant or deny the wrong permission, which is the
// one class of bug an access control system must never have.
func TestPermissionBitsMatchTheAPI(t *testing.T) {
	cases := []struct {
		perm v1.Permission
		bit  PermMask
	}{
		{v1.Permission_PERMISSION_VIEW, PermView},
		{v1.Permission_PERMISSION_DOWNLOAD, PermDownload},
		{v1.Permission_PERMISSION_UPLOAD, PermUpload},
		{v1.Permission_PERMISSION_EDIT, PermEdit},
		{v1.Permission_PERMISSION_DELETE, PermDelete},
		{v1.Permission_PERMISSION_TRASH_MANAGE, PermTrashManage},
		{v1.Permission_PERMISSION_SHARE, PermShare},
		{v1.Permission_PERMISSION_ACL_MANAGE, PermAclManage},
		{v1.Permission_PERMISSION_USER_MANAGE, PermUserManage},
		{v1.Permission_PERMISSION_AUDIT_READ, PermAuditRead},
		{v1.Permission_PERMISSION_STORAGE_MANAGE, PermStorageManage},
		{v1.Permission_PERMISSION_SYSTEM_MANAGE, PermSystemManage},
	}
	seen := PermMask(0)
	for _, tc := range cases {
		if got := permBit(tc.perm); got != tc.bit {
			t.Errorf("%s: bit = %d, want %d", tc.perm, got, tc.bit)
		}
		if seen.Has(tc.bit) {
			t.Errorf("%s: bit %d collides with an earlier permission", tc.perm, tc.bit)
		}
		seen |= tc.bit
	}
	if seen != PermAll {
		t.Errorf("the catalog covers %b but PermAll is %b", seen, PermAll)
	}
	// The mask must round trip through the enum list a client sends.
	for _, perm := range []v1.Permission{v1.Permission_PERMISSION_VIEW, v1.Permission_PERMISSION_UPLOAD} {
		if !PermMaskFrom([]v1.Permission{perm}).Has(permBit(perm)) {
			t.Errorf("PermMaskFrom lost %s", perm)
		}
	}
	if got := PermMaskFrom([]v1.Permission{v1.Permission_PERMISSION_VIEW, v1.Permission_PERMISSION_DOWNLOAD}); got != 3 {
		t.Errorf("view|download = %d, want 3", got)
	}
	// PermissionValues must render the mask back into enum values.
	values := (PermView | PermDownload).PermissionValues()
	if len(values) != 2 || values[0] != v1.Permission_PERMISSION_VIEW || values[1] != v1.Permission_PERMISSION_DOWNLOAD {
		t.Errorf("PermissionValues = %v", values)
	}
}

func TestRolePresetsKeepGuestReadOnly(t *testing.T) {
	guest := DefaultPermsFor(RoleGuest)
	if guest != PermView|PermDownload {
		t.Fatalf("guest preset = %b, want view|download", guest)
	}
	if guest.HasAny(PermUpload | PermEdit | PermDelete | PermUserManage | PermSystemManage) {
		t.Errorf("the guest preset grants a write permission: %b", guest)
	}
	user := DefaultPermsFor(RoleUser)
	if user.HasAny(PermUserManage | PermSystemManage | PermStorageManage) {
		t.Errorf("the user preset grants an administrative permission: %b", user)
	}
	if !user.Has(PermUpload | PermEdit | PermDelete | PermAclManage) {
		t.Errorf("the user preset is missing a content permission: %b", user)
	}
	manager := DefaultPermsFor(RoleManager)
	if !manager.Has(PermUserManage) {
		t.Errorf("the manager preset cannot manage accounts: %b", manager)
	}
	if manager.Has(PermSystemManage) {
		t.Errorf("the manager preset must not carry system management: %b", manager)
	}
	if PermMaskFrom([]v1.Permission{PermissionEntries()[0].Permission}) == 0 {
		t.Errorf("the permission catalog is empty")
	}
}

func TestEvaluateOwnerKeepsAccountCeiling(t *testing.T) {
	owner := uuid.Must(uuid.NewV7())
	node := folder(uuid.Must(uuid.NewV7()), owner, VisibilityPrivate)

	full := account(owner, RoleUser, RankUser, PermNodeScope)
	access := EvaluateNodeAccess(NodeAccessInput{Caller: full, Node: node, AclByNode: nil})
	if !access.Owned {
		t.Fatalf("the owner is not recognised")
	}
	if !access.Perms.Has(PermNodeScope) {
		t.Errorf("the owner lost a node permission: %b", access.Perms)
	}

	// Revoking upload from the account also removes it from the owner's own
	// folders, which is what makes a permission change take effect at once.
	limited := account(owner, RoleGuest, RankGuest, PermView|PermDownload)
	access = EvaluateNodeAccess(NodeAccessInput{Caller: limited, Node: node, AclByNode: nil})
	if access.Perms.Has(PermUpload | PermEdit | PermDelete) {
		t.Errorf("the owner kept a permission the account lost: %b", access.Perms)
	}
}

func TestEvaluateDenyBeatsAllow(t *testing.T) {
	owner := uuid.Must(uuid.NewV7())
	viewer := uuid.Must(uuid.NewV7())
	node := folder(uuid.Must(uuid.NewV7()), owner, VisibilityPrivate)
	caller := account(viewer, RoleUser, RankUser, PermAll)

	acl := map[uuid.UUID][]*AclEntry{
		node.ID: {
			allow(SubjectTypeUser, viewer.String(), PermView|PermDownload|PermUpload, true),
			deny(SubjectTypeUser, viewer.String(), PermDownload, true),
		},
	}
	access := EvaluateNodeAccess(NodeAccessInput{Caller: caller, Node: node, AclByNode: acl})
	if !access.Perms.Has(PermView) {
		t.Errorf("view was not granted: %b", access.Perms)
	}
	if access.Perms.Has(PermDownload) {
		t.Errorf("a deny entry did not beat an allow entry: %b", access.Perms)
	}
	if !access.Perms.Has(PermUpload) {
		t.Errorf("the deny removed a permission it did not cover: %b", access.Perms)
	}
}

func TestEvaluateAncestorInheritance(t *testing.T) {
	owner := uuid.Must(uuid.NewV7())
	viewer := uuid.Must(uuid.NewV7())
	parent := folder(uuid.Must(uuid.NewV7()), owner, VisibilityPrivate)
	child := folder(uuid.Must(uuid.NewV7()), owner, VisibilityPrivate)
	caller := account(viewer, RoleUser, RankUser, PermAll)

	// A non inheriting allow stops at the folder it is attached to.
	acl := map[uuid.UUID][]*AclEntry{
		parent.ID: {allow(SubjectTypeUser, viewer.String(), PermView, false)},
	}
	access := EvaluateNodeAccess(NodeAccessInput{
		Caller: caller, Node: child, Ancestors: []*Node{parent}, AclByNode: acl,
	})
	if access.Perms.Has(PermView) {
		t.Errorf("a non inheriting allow reached a child: %b", access.Perms)
	}

	// An inheriting allow reaches the whole subtree.
	acl[parent.ID] = []*AclEntry{allow(SubjectTypeUser, viewer.String(), PermView, true)}
	access = EvaluateNodeAccess(NodeAccessInput{
		Caller: caller, Node: child, Ancestors: []*Node{parent}, AclByNode: acl,
	})
	if !access.Perms.Has(PermView) {
		t.Errorf("an inheriting allow did not reach a child: %b", access.Perms)
	}

	// A deny reaches the subtree even when the entry is not inheritable.
	acl[parent.ID] = []*AclEntry{
		allow(SubjectTypeUser, viewer.String(), PermView|PermUpload, true),
		deny(SubjectTypeUser, viewer.String(), PermUpload, false),
	}
	access = EvaluateNodeAccess(NodeAccessInput{
		Caller: caller, Node: child, Ancestors: []*Node{parent}, AclByNode: acl,
	})
	if access.Perms.Has(PermUpload) {
		t.Errorf("a deny on the parent did not reach the child: %b", access.Perms)
	}
	if !access.Perms.Has(PermView) {
		t.Errorf("the deny removed an unrelated permission: %b", access.Perms)
	}
}

func TestEvaluateRoleAndEveryoneEntries(t *testing.T) {
	owner := uuid.Must(uuid.NewV7())
	manager := uuid.Must(uuid.NewV7())
	node := folder(uuid.Must(uuid.NewV7()), owner, VisibilityPrivate)
	caller := account(manager, RoleManager, RankManager, PermAll)

	acl := map[uuid.UUID][]*AclEntry{
		node.ID: {
			allow(SubjectTypeRole, "manager", PermView|PermAclManage, true),
			deny(SubjectTypeEveryone, "", PermDelete, true),
		},
	}
	access := EvaluateNodeAccess(NodeAccessInput{Caller: caller, Node: node, AclByNode: acl})
	if !access.Perms.Has(PermView | PermAclManage) {
		t.Errorf("a role entry did not apply: %b", access.Perms)
	}
	if access.Perms.Has(PermDelete) {
		t.Errorf("the everyone deny did not apply: %b", access.Perms)
	}

	// An anonymous caller is only covered by an everyone entry.
	access = EvaluateNodeAccess(NodeAccessInput{Caller: AnonymousCaller(), Node: node, AclByNode: acl})
	if access.Perms.Has(PermView) {
		t.Errorf("an anonymous caller matched a role entry: %b", access.Perms)
	}
}

func TestEvaluateVisibility(t *testing.T) {
	owner := uuid.Must(uuid.NewV7())
	viewer := uuid.Must(uuid.NewV7())
	caller := account(viewer, RoleUser, RankUser, PermAll)

	priv := file(uuid.Must(uuid.NewV7()), owner, VisibilityPrivate)
	if got := EvaluateNodeAccess(NodeAccessInput{Caller: caller, Node: priv}); got.Perms.Has(PermView) {
		t.Errorf("a private node is visible to a stranger: %b", got.Perms)
	}

	internal := file(uuid.Must(uuid.NewV7()), owner, VisibilityInternal)
	if got := EvaluateNodeAccess(NodeAccessInput{Caller: caller, Node: internal}); !got.Perms.Has(PermView | PermDownload) {
		t.Errorf("an internal node is not readable by an authenticated caller: %b", got.Perms)
	}

	public := file(uuid.Must(uuid.NewV7()), owner, VisibilityPublic)
	if got := EvaluateNodeAccess(NodeAccessInput{Caller: AnonymousCaller(), Node: public}); !got.Perms.Has(PermView | PermDownload) {
		t.Errorf("a public node is not readable anonymously: %b", got.Perms)
	}

	// A private parent makes a public child unreachable.
	parent := folder(uuid.Must(uuid.NewV7()), owner, VisibilityPrivate)
	child := file(uuid.Must(uuid.NewV7()), owner, VisibilityPublic)
	if got := EvaluateNodeAccess(NodeAccessInput{Caller: caller, Node: child, Ancestors: []*Node{parent}}); got.Perms.Has(PermView) {
		t.Errorf("a public child of a private folder leaked: %b", got.Perms)
	}
}

func TestEvaluateFolderPassword(t *testing.T) {
	owner := uuid.Must(uuid.NewV7())
	viewer := uuid.Must(uuid.NewV7())
	protected := folder(uuid.Must(uuid.NewV7()), owner, VisibilityPrivate)
	protected.PasswordHash = "hash"
	child := file(uuid.Must(uuid.NewV7()), owner, VisibilityPrivate)
	caller := account(viewer, RoleUser, RankUser, PermAll)

	acl := map[uuid.UUID][]*AclEntry{
		protected.ID: {allow(SubjectTypeEveryone, "", PermNodeScope, true)},
	}
	access := EvaluateNodeAccess(NodeAccessInput{
		Caller: caller, Node: protected, AclByNode: acl,
	})
	if !access.Locked || access.LockedBy != protected.ID {
		t.Fatalf("a password protected folder is not locked: %+v", access)
	}
	if access.Perms != PermView {
		t.Errorf("a locked folder grants %b, want view only", access.Perms)
	}

	// The lock also covers the subtree.
	access = EvaluateNodeAccess(NodeAccessInput{
		Caller: caller, Node: child, Ancestors: []*Node{protected}, AclByNode: acl,
	})
	if !access.Locked {
		t.Errorf("the lock did not cover the subtree")
	}

	// Proving knowledge of the password lifts it.
	caller.Unlocked[protected.ID] = true
	access = EvaluateNodeAccess(NodeAccessInput{
		Caller: caller, Node: child, Ancestors: []*Node{protected}, AclByNode: acl,
	})
	if access.Locked {
		t.Errorf("an unlocked folder is still locked")
	}
	if !access.Perms.Has(PermDownload | PermUpload) {
		t.Errorf("the unlock did not restore the acl permissions: %b", access.Perms)
	}

	// The owner never has to unlock its own folder.
	ownerCaller := account(owner, RoleUser, RankUser, PermNodeScope)
	access = EvaluateNodeAccess(NodeAccessInput{
		Caller: ownerCaller, Node: protected, AclByNode: acl,
	})
	if access.Locked {
		t.Errorf("the owner has to unlock its own folder")
	}
}

func TestCanManageUserFollowsRank(t *testing.T) {
	adminCaller := account(uuid.Must(uuid.NewV7()), RoleAdmin, RankAdmin, PermAll)
	managerCaller := account(uuid.Must(uuid.NewV7()), RoleManager, RankManager, PermAll)
	userCaller := account(uuid.Must(uuid.NewV7()), RoleUser, RankUser, PermAll)

	admin := &User{ID: uuid.Must(uuid.NewV7()), Role: RoleAdmin, Rank: RankAdmin}
	manager := &User{ID: uuid.Must(uuid.NewV7()), Role: RoleManager, Rank: RankManager}
	lower := &User{ID: uuid.Must(uuid.NewV7()), Role: RoleUser, Rank: 200}
	guest := &User{ID: uuid.Must(uuid.NewV7()), Role: RoleGuest, Rank: RankGuest}

	if err := CanManageUser(adminCaller, manager); err != nil {
		t.Errorf("an admin cannot manage a promoted manager: %v", err)
	}
	if err := CanManageUser(adminCaller, lower); err != nil {
		t.Errorf("an admin cannot manage a user: %v", err)
	}
	if err := CanManageUser(managerCaller, manager); err == nil {
		t.Errorf("a manager managed an account of equal rank")
	}
	if err := CanManageUser(managerCaller, admin); err == nil {
		t.Errorf("a manager managed the built-in administrator")
	}
	if err := CanManageUser(managerCaller, lower); err != nil {
		t.Errorf("a manager cannot manage a lower ranked account: %v", err)
	}
	if err := CanManageUser(userCaller, guest); err != nil {
		t.Errorf("a user cannot manage a guest: %v", err)
	}
	if err := CanManageUser(userCaller, lower); err == nil {
		t.Errorf("a user managed a peer")
	}
	// Nobody manages itself through this path.
	if err := CanManageUser(adminCaller, &User{ID: adminCaller.UserID, Rank: 1}); err == nil {
		t.Errorf("an account managed itself")
	}
	if err := CanManageUser(AnonymousCaller(), guest); err == nil {
		t.Errorf("an anonymous caller managed an account")
	}
}

func TestChildPathAndDisplayPath(t *testing.T) {
	root := uuid.Must(uuid.NewV7())
	child := uuid.Must(uuid.NewV7())
	leaf := uuid.Must(uuid.NewV7())

	rootPath := ChildPath("/", root)
	if rootPath != "/"+root.String()+"/" {
		t.Fatalf("root path = %q", rootPath)
	}
	childPath := ChildPath(rootPath, child)
	if childPath != rootPath+child.String()+"/" {
		t.Fatalf("child path = %q", childPath)
	}
	if got := ChildPath(childPath, leaf); got != childPath+leaf.String()+"/" {
		t.Fatalf("leaf path = %q", got)
	}
	// A malformed parent path is repaired rather than producing a broken key.
	if got := ChildPath("", root); got != "/"+root.String()+"/" {
		t.Errorf("empty parent path produced %q", got)
	}

	ancestors := []*Node{{Name: "documents"}, {Name: "y2026"}}
	if got := DisplayPathOf(ancestors, &Node{Name: "q1.pdf"}, "我的网盘"); got != "/我的网盘/documents/y2026/q1.pdf" {
		t.Errorf("display path = %q", got)
	}
	if got := DisplayPathOf(nil, &Node{Name: "root.txt"}, ""); got != "/我的网盘/root.txt" {
		t.Errorf("display path with the default root = %q", got)
	}
}

func TestShareGrantStaysInsideShareScope(t *testing.T) {
	node := file(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), VisibilityPrivate)
	node.PasswordHash = "hash"

	// A share can never convey an administrative permission even if the
	// creator holds it.
	grant := ShareGrant(PermView|PermDownload|PermUserManage|PermSystemManage, true, node)
	if grant.Perms.Has(PermUserManage | PermSystemManage) {
		t.Errorf("a share conveyed an administrative permission: %b", grant.Perms)
	}
	if !grant.Perms.Has(PermView | PermDownload) {
		t.Errorf("a share lost a content permission: %b", grant.Perms)
	}

	locked := ShareGrant(PermView|PermDownload, false, node)
	if !locked.Locked || locked.Perms.Has(PermDownload) {
		t.Errorf("an unlocked share leaked the download permission: %+v", locked)
	}
}

func TestUploadLimitsNormalisation(t *testing.T) {
	limits := UploadLimits{MinChunkSize: 5 << 20, MaxChunkSize: 5 << 30}
	if got := normalizeChunk(1, limits); got != 5<<20 {
		t.Errorf("chunk below the minimum = %d", got)
	}
	if got := normalizeChunk(1<<40, limits); got != 5<<30 {
		t.Errorf("chunk above the maximum = %d", got)
	}
	if got := normalizeChunk(5<<20+1, limits); got != 5<<20+256<<10 {
		t.Errorf("chunk was not aligned = %d", got)
	}
	if got := Extension("Report.PDF"); got != "pdf" {
		t.Errorf("extension = %q", got)
	}
	if got := Extension("archive.tar.gz"); got != "gz" {
		t.Errorf("extension = %q", got)
	}
	if got := DetectMimeType("photo.png"); got != "image/png" {
		t.Errorf("mime type = %q", got)
	}
	if !IsPreviewable("image/png") || IsPreviewable("application/zip") {
		t.Errorf("preview detection is wrong")
	}
	if got := ObjectKey(uuid.Nil, uuid.Nil, 0); got != "files/00000000-0000-0000-0000-000000000000/00000000-0000-0000-0000-000000000000" {
		t.Errorf("object key = %q", got)
	}
}
