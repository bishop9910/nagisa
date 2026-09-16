package biz

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type callerKey struct{}

// Caller is the authenticated identity behind a request. It is attached to the
// context by the transport middleware and read by the usecases, so authorising
// a call never depends on a handler remembering to pass the identity along.
type Caller struct {
	UserID    uuid.UUID
	Username  string
	Nickname  string
	Role      Role
	Rank      int32
	Perms     PermMask
	Status    UserStatus
	Anonymous bool

	// ShareToken is set when the request acts through a public share link
	// instead of an account. Perms then carries the capability set of the
	// share rather than the permissions of an account.
	ShareToken string
	ShareID    uuid.UUID

	// Unlocked holds the ids of password protected nodes the caller proved
	// knowledge of, including every node covered by an X-Node-Token.
	Unlocked map[uuid.UUID]bool

	IP        string
	UserAgent string
	RequestID string
}

// AnonymousCaller returns the identity used for unauthenticated requests.
func AnonymousCaller() *Caller {
	return &Caller{Anonymous: true, Role: RoleGuest, Rank: 0}
}

// NewContext stores the caller in the context.
func NewContext(ctx context.Context, c *Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFromContext returns the caller attached to the context, or the
// anonymous identity when the request carried no credential.
func CallerFromContext(ctx context.Context) *Caller {
	if c, ok := ctx.Value(callerKey{}).(*Caller); ok && c != nil {
		return c
	}
	return AnonymousCaller()
}

// IsAuthenticated reports whether the caller carries an account.
func (c *Caller) IsAuthenticated() bool {
	return c != nil && !c.Anonymous && c.UserID != uuid.Nil
}

// Has reports whether the caller holds every permission in want.
func (c *Caller) Has(want PermMask) bool {
	return c != nil && !c.Anonymous && c.Perms.Has(want)
}

// HasAny reports whether the caller holds at least one permission in want.
func (c *Caller) HasAny(want PermMask) bool {
	return c != nil && !c.Anonymous && c.Perms.HasAny(want)
}

// Require returns ErrUnauthenticated or ErrPermissionDenied when the caller
// does not hold want.
func (c *Caller) Require(want PermMask) error {
	if !c.IsAuthenticated() {
		return ErrUnauthenticated
	}
	if !c.Perms.Has(want) {
		return ErrPermissionDenied
	}
	return nil
}

// IsUnlocked reports whether a node was unlocked for this caller.
func (c *Caller) IsUnlocked(nodeID uuid.UUID) bool {
	if c == nil || c.Unlocked == nil {
		return false
	}
	return c.Unlocked[nodeID]
}

// NodeAccess is the outcome of evaluating the access rules for one node.
type NodeAccess struct {
	// Permissions the caller effectively holds on the node.
	Perms PermMask
	// Locked reports that a password protected node has not been unlocked.
	Locked bool
	// LockedBy identifies the outermost locked node in the chain.
	LockedBy uuid.UUID
	// Owned reports that the caller owns the node.
	Owned bool
	// Visibility is the effective visibility after ancestor propagation.
	Visibility Visibility
}

// NodeAccessInput bundles everything the evaluation needs. Ancestors must be
// ordered from the root down to, but excluding, the node.
type NodeAccessInput struct {
	Caller    *Caller
	Node      *Node
	Ancestors []*Node
	// AclByNode holds the entries attached to each node in the chain. Entries
	// for the node itself are also looked up here.
	AclByNode map[uuid.UUID][]*AclEntry
}

// EvaluateNodeAccess applies the documented access order:
// owner, then deny, then allow, then visibility, then the account permission
// set, and finally the folder password.
func EvaluateNodeAccess(in NodeAccessInput) NodeAccess {
	node := in.Node
	if node == nil {
		return NodeAccess{}
	}
	caller := in.Caller
	acc := NodeAccess{Visibility: effectiveVisibility(node, in.Ancestors)}

	if caller == nil {
		caller = AnonymousCaller()
	}

	// A password protected folder hides its subtree even from an allowed
	// caller until the password is supplied.
	for _, n := range in.Ancestors {
		if n.PasswordHash != "" && !caller.IsUnlocked(n.ID) {
			acc.Locked = true
			acc.LockedBy = n.ID
			break
		}
	}
	if !acc.Locked && node.PasswordHash != "" && !caller.IsUnlocked(node.ID) {
		acc.Locked = true
		acc.LockedBy = node.ID
	}

	owned := caller.IsAuthenticated() && node.OwnerID == caller.UserID
	acc.Owned = owned

	// The owner disposes of every node level permission on its own nodes, and
	// is never asked to unlock a password it set itself. The account
	// permission set still caps the result, so revoking upload from an account
	// also removes it from the account's own folders.
	if owned {
		acc.Locked = false
		acc.Perms = PermNodeScope.And(caller.Perms)
		return acc
	}

	granted := PermNone
	if caller.IsAuthenticated() {
		switch acc.Visibility {
		case VisibilityPublic:
			granted = PermView | PermDownload
		case VisibilityInternal:
			granted = PermView | PermDownload
		}
	} else if acc.Visibility == VisibilityPublic {
		granted = PermView | PermDownload
	}

	allowed := PermNone
	denied := PermNone

	// Entries on the node itself always apply.
	for _, e := range in.AclByNode[node.ID] {
		if !aclMatches(e, caller) {
			continue
		}
		if e.Effect == EffectDeny {
			denied |= e.Permissions
		} else {
			allowed |= e.Permissions
		}
	}
	// Ancestor entries apply to the subtree. A deny always reaches down, an
	// allow only when the entry is marked inheritable.
	for _, a := range in.Ancestors {
		for _, e := range in.AclByNode[a.ID] {
			if !aclMatches(e, caller) {
				continue
			}
			if e.Effect == EffectDeny {
				denied |= e.Permissions
			} else if e.Inherit {
				allowed |= e.Permissions
			}
		}
	}

	perms := (granted | allowed) &^ denied
	// An anonymous caller has no account permission set, so the ceiling is the
	// read-only baseline a guest is granted. Without it a public node would
	// resolve to no permission at all and become unreadable.
	ceiling := caller.Perms
	if !caller.IsAuthenticated() {
		ceiling = DefaultPermsFor(RoleGuest)
	}
	perms = perms.And(PermNodeScope).And(ceiling)
	if acc.Locked {
		perms = perms.And(PermView)
	}
	acc.Perms = perms
	return acc
}

// effectiveVisibility folds the ancestor chain into a single visibility. The
// most restrictive node wins, so a private folder can never be reached through
// a public child.
func effectiveVisibility(node *Node, ancestors []*Node) Visibility {
	vis := node.Visibility
	if vis == VisibilityUnspecified {
		vis = VisibilityPrivate
	}
	for _, a := range ancestors {
		v := a.Visibility
		if v == VisibilityUnspecified {
			v = VisibilityPrivate
		}
		if v < vis {
			vis = v
		}
	}
	return vis
}

// aclMatches reports whether an entry applies to the caller.
func aclMatches(e *AclEntry, caller *Caller) bool {
	if e == nil || e.Permissions.IsZero() {
		return false
	}
	switch e.SubjectType {
	case SubjectTypeEveryone:
		return true
	case SubjectTypeUser:
		return caller.IsAuthenticated() && e.SubjectID == caller.UserID.String()
	case SubjectTypeRole:
		if !caller.IsAuthenticated() {
			return false
		}
		return strings.EqualFold(e.SubjectID, RoleName(caller.Role))
	default:
		return false
	}
}

// RoleName returns the machine name of a role, which is also the value stored
// in a SUBJECT_TYPE_ROLE ACL entry.
func RoleName(r Role) string {
	switch r {
	case RoleAdmin:
		return "admin"
	case RoleManager:
		return "manager"
	case RoleUser:
		return "user"
	case RoleGuest:
		return "guest"
	default:
		return ""
	}
}

// ShareGrant computes the permissions a share link conveys on a node. Share
// access never consults the node ACL of the creator, only the share capability
// set, the node lifecycle state and the share password.
func ShareGrant(perms PermMask, unlocked bool, node *Node) NodeAccess {
	acc := NodeAccess{Perms: perms.And(PermShareScope), Visibility: node.Visibility}
	if node.PasswordHash != "" && !unlocked {
		acc.Locked = true
		acc.LockedBy = node.ID
	}
	if acc.Locked {
		acc.Perms = acc.Perms.And(PermView)
	}
	return acc
}
