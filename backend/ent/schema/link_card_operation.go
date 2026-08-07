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

// LinkCardOperation provides permanent idempotency for financial mutations.
type LinkCardOperation struct{ ent.Schema }

func (LinkCardOperation) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "link_card_operations"}}
}

func (LinkCardOperation) Fields() []ent.Field {
	return []ent.Field{
		field.String("scope").MaxLen(64),
		field.Int64("actor_user_id"),
		field.Int64("creator_user_id"),
		field.Int64("api_key_id").Optional().Nillable(),
		field.String("idempotency_key_hash").MaxLen(64),
		field.String("request_fingerprint").MaxLen(64),
		field.JSON("response_body", map[string]any{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (LinkCardOperation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope", "actor_user_id", "idempotency_key_hash").Unique(),
		index.Fields("api_key_id", "created_at"),
	}
}
