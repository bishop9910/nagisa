package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"nagisa/internal/biz"
)

// Share holds the schema definition for one capability link.
type Share struct {
	ent.Schema
}

// Mixin of the Share.
func (Share) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the Share.
func (Share) Fields() []ent.Field {
	return []ent.Field{
		field.String("token").MaxLen(64).NotEmpty(),
		field.UUID("node_id", uuid.Nil).Default(newZeroUUID),
		field.UUID("owner_id", uuid.Nil).Default(newZeroUUID),
		field.String("name").Default(""),
		field.String("description").Default(""),
		field.Int64("permissions").GoType(biz.PermMask(0)).Default(0),
		field.String("password_hash").Default(""),
		field.String("password_hint").Default(""),
		field.Time("expires_at").Optional().Nillable(),
		field.Int64("max_downloads").Default(0),
		field.Int64("download_count").Default(0),
		field.Int64("view_count").Default(0),
		field.Int32("status").GoType(biz.ShareStatus(0)).Default(int32(biz.ShareStatusActive)),
		field.UUID("created_by", uuid.Nil).Default(newZeroUUID),
	}
}

// Indexes of the Share.
func (Share) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token").Unique(),
		index.Fields("node_id"),
		index.Fields("owner_id", "status"),
		index.Fields("expires_at"),
	}
}

// RefreshSession holds the schema definition for one refresh token.
type RefreshSession struct {
	ent.Schema
}

// Mixin of the RefreshSession.
func (RefreshSession) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the RefreshSession.
func (RefreshSession) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("user_id", uuid.Nil).Default(newZeroUUID),
		// Only the hash of a refresh token is stored, so a database leak
		// cannot be replayed against the API.
		field.String("token_hash").MaxLen(128).NotEmpty(),
		field.String("ip").Default(""),
		field.String("user_agent").Default(""),
		field.Time("expires_at"),
		field.Time("revoked_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
	}
}

// Indexes of the RefreshSession.
func (RefreshSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token_hash").Unique(),
		index.Fields("user_id", "expires_at"),
	}
}

// Setting holds the schema definition for a runtime tunable.
type Setting struct {
	ent.Schema
}

// Mixin of the Setting.
func (Setting) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the Setting.
func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").MaxLen(128).NotEmpty(),
		field.String("value").Default(""),
	}
}

// Indexes of the Setting.
func (Setting) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("key").Unique(),
	}
}

// AuditLog holds the schema definition for one audit entry. The table is
// append only: rows are created and aggregated, never updated.
type AuditLog struct {
	ent.Schema
}

// Mixin of the AuditLog.
func (AuditLog) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}}
}

// Fields of the AuditLog.
func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("actor_id", uuid.Nil).Default(newZeroUUID),
		field.String("actor_name").Default(""),
		field.String("action").MaxLen(128).Default(""),
		field.String("target_type").Default(""),
		field.String("target_id").Default(""),
		field.String("target_name").Default(""),
		field.Bool("success").Default(true),
		field.String("error_reason").Default(""),
		field.JSON("detail", map[string]string{}).Optional(),
		field.String("ip").Default(""),
		field.String("user_agent").Default(""),
		field.String("request_id").Default(""),
		field.Time("created_at").Immutable().Default(time.Now),
	}
}

// Indexes of the AuditLog.
func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("actor_id", "created_at"),
		index.Fields("action", "created_at"),
		index.Fields("target_type", "target_id"),
		index.Fields("success"),
	}
}
