package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"nagisa/internal/biz"
)

// Node holds the schema definition for a file or a folder. Both kinds share
// one table so description, ACL, visibility and password behave identically.
type Node struct {
	ent.Schema
}

// Mixin of the Node.
func (Node) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the Node.
func (Node) Fields() []ent.Field {
	return []ent.Field{
		// The zero id marks a root level node, which keeps every query a plain
		// equality check instead of a null test.
		field.UUID("parent_id", uuid.Nil).Default(newZeroUUID),
		field.String("name").MaxLen(255).NotEmpty(),
		// name_lower mirrors name under the configured case sensitivity so a
		// sibling collision is a single indexed lookup.
		field.String("name_lower").MaxLen(255).Default(""),
		field.Int32("kind").GoType(biz.NodeKind(0)).Default(int32(biz.NodeKindFile)),
		field.UUID("owner_id", uuid.Nil).Default(newZeroUUID),
		field.Int64("size").Default(0),
		field.String("mime_type").Default(""),
		field.String("extension").Default(""),
		field.String("etag").Default(""),
		field.String("storage_key").Default(""),
		field.Int32("status").GoType(biz.NodeStatus(0)).Default(int32(biz.NodeStatusActive)),
		field.String("description").Default(""),
		field.Int32("visibility").GoType(biz.Visibility(0)).Default(int32(biz.VisibilityPrivate)),
		field.String("password_hash").Default(""),
		field.String("password_hint").Default(""),
		field.Bool("has_thumbnail").Default(false),
		field.JSON("metadata", map[string]string{}).Optional(),
		// path is the materialised chain of ancestor ids, for example
		// "/<id1>/<id2>/", which turns a subtree into one prefix scan.
		field.String("path").Default("/"),
		field.Int32("depth").Default(0),
		field.Int64("child_count").Default(0),
		field.Int64("file_count").Default(0),
		field.Int64("folder_count").Default(0),
		field.Int64("subtree_size").Default(0),
		field.UUID("current_version_id", uuid.Nil).Optional().Nillable(),
		field.Int32("version_count").Default(0),
		field.Time("trashed_at").Optional().Nillable(),
		field.UUID("original_parent_id", uuid.Nil).Default(newZeroUUID),
		field.UUID("created_by", uuid.Nil).Default(newZeroUUID),
		field.UUID("updated_by", uuid.Nil).Default(newZeroUUID),
	}
}

// Indexes of the Node.
func (Node) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("parent_id", "name_lower"),
		index.Fields("path"),
		index.Fields("owner_id", "status"),
		index.Fields("owner_id", "kind"),
		index.Fields("status"),
		index.Fields("storage_key"),
		index.Fields("trashed_at"),
	}
}

// NodeAcl holds the schema definition for one access control entry.
type NodeAcl struct {
	ent.Schema
}

// Mixin of the NodeAcl.
func (NodeAcl) Mixin() []ent.Mixin {
	return []ent.Mixin{IDMixin{}, TimeMixin{}}
}

// Fields of the NodeAcl.
func (NodeAcl) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("node_id", uuid.Nil).Default(newZeroUUID),
		field.Int32("subject_type").GoType(biz.SubjectType(0)).Default(int32(biz.SubjectTypeUser)),
		field.String("subject_id").Default(""),
		field.Int32("effect").GoType(biz.Effect(0)).Default(int32(biz.EffectAllow)),
		field.Int64("permissions").GoType(biz.PermMask(0)).Default(0),
		field.Bool("inherit").Default(true),
		field.UUID("created_by", uuid.Nil).Default(newZeroUUID),
	}
}

// Indexes of the NodeAcl.
func (NodeAcl) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("node_id", "subject_type", "subject_id").Unique(),
	}
}
