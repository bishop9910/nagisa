package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"nagisa/internal/biz"
)

// Upload holds the schema definition for one multipart upload session.
type Upload struct {
	ent.Schema
}

// Mixin of the Upload.
func (Upload) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the Upload.
func (Upload) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("parent_id", uuid.Nil).Default(newZeroUUID),
		field.String("name").MaxLen(255).Default(""),
		field.String("name_lower").MaxLen(255).Default(""),
		field.UUID("owner_id", uuid.Nil).Default(newZeroUUID),
		field.Int64("size").Default(0),
		field.String("mime_type").Default(""),
		field.Int64("chunk_size").Default(0),
		field.Int32("total_parts").Default(0),
		field.Int32("status").GoType(biz.UploadStatus(0)).Default(int32(biz.UploadStatusPending)),
		field.Int32("mode").GoType(biz.UploadMode(0)).Default(int32(biz.UploadModePresigned)),
		field.Int32("policy").GoType(biz.ConflictPolicy(0)).Default(int32(biz.ConflictPolicyRename)),
		// path is the staging prefix every part of this session is stored
		// under, which makes an abort a single prefix removal.
		field.String("path").Default(""),
		field.String("etag").Default(""),
		field.UUID("node_id", uuid.Nil).Optional().Nillable(),
		field.String("description").Default(""),
		field.JSON("metadata", map[string]string{}).Optional(),
		field.Int64("received_bytes").Default(0),
		field.String("last_error").Default(""),
		field.Time("expires_at"),
	}
}

// Indexes of the Upload.
func (Upload) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("owner_id", "status"),
		index.Fields("owner_id", "parent_id", "name_lower", "status"),
		index.Fields("expires_at"),
	}
}

// UploadPart holds the schema definition for one stored part.
type UploadPart struct {
	ent.Schema
}

// Mixin of the UploadPart.
func (UploadPart) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the UploadPart.
func (UploadPart) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("upload_id", uuid.Nil).Default(newZeroUUID),
		field.Int32("part_number").Default(0),
		field.Int64("size").Default(0),
		field.String("etag").Default(""),
		field.String("storage_key").Default(""),
	}
}

// Indexes of the UploadPart.
func (UploadPart) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("upload_id", "part_number").Unique(),
	}
}

// NodeVersion holds the schema definition for one retained content revision.
type NodeVersion struct {
	ent.Schema
}

// Mixin of the NodeVersion.
func (NodeVersion) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the NodeVersion.
func (NodeVersion) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("node_id", uuid.Nil).Default(newZeroUUID),
		field.Int32("version").Default(1),
		field.String("storage_key").Default(""),
		field.Int64("size").Default(0),
		field.String("mime_type").Default(""),
		field.String("etag").Default(""),
		field.String("comment").Default(""),
		field.UUID("created_by", uuid.Nil).Default(newZeroUUID),
	}
}

// Indexes of the NodeVersion.
func (NodeVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("node_id", "version").Unique(),
		index.Fields("storage_key"),
	}
}

// PendingDeletion holds objects that must be released after a commit. Queueing
// the deletion makes object removal retryable instead of best effort.
type PendingDeletion struct {
	ent.Schema
}

// Mixin of the PendingDeletion.
func (PendingDeletion) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the PendingDeletion.
func (PendingDeletion) Fields() []ent.Field {
	return []ent.Field{
		field.String("storage_key").Default(""),
		field.String("reason").Default(""),
		field.Int32("attempts").Default(0),
		field.String("last_error").Default(""),
	}
}

// Indexes of the PendingDeletion.
func (PendingDeletion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
	}
}
