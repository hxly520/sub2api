package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// LinkCardGroupAuthorization is the administrator-managed allowlist of groups
// that registered users may bind to newly issued prepaid link keys.
type LinkCardGroupAuthorization struct{ ent.Schema }

func (LinkCardGroupAuthorization) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "link_card_group_authorizations"}}
}

func (LinkCardGroupAuthorization) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("group_id").Unique(),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.Int64("created_by").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (LinkCardGroupAuthorization) Indexes() []ent.Index {
	return []ent.Index{index.Fields("enabled", "sort_order", "group_id")}
}
