package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"nagisa/internal/biz"
)

// User holds the schema definition for an account.
type User struct {
	ent.Schema
}

// Mixin of the User.
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("username").MaxLen(64).NotEmpty(),
		field.String("nickname").Default(""),
		field.String("email").Default(""),
		field.String("avatar_url").Default(""),
		field.String("password_hash").Default(""),
		// role selects the preset template, rank decides authority. Both are
		// bound to the domain types so the two cannot drift apart.
		field.Int32("role").GoType(biz.Role(0)).Default(int32(biz.RoleGuest)),
		field.Int32("rank").Default(0),
		field.Int64("permissions").GoType(biz.PermMask(0)).Default(0),
		field.Int32("status").GoType(biz.UserStatus(0)).Default(int32(biz.UserStatusActive)),
		field.Int64("quota_bytes").Default(0),
		field.Int64("used_bytes").Default(0),
		field.Int64("file_count").Default(0),
		field.Int64("folder_count").Default(0),
		field.String("remark").Default(""),
		field.Bool("must_change_password").Default(false),
		field.Time("last_login_at").Optional().Nillable(),
		field.UUID("created_by", uuid.Nil).Default(newZeroUUID),
	}
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("username").Unique(),
		index.Fields("status"),
		index.Fields("role"),
		index.Fields("rank"),
	}
}
