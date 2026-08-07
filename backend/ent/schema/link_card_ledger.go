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

// LinkCardLedger is an append-only money and prepaid-reserve journal.
type LinkCardLedger struct{ ent.Schema }

func (LinkCardLedger) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "link_card_ledger"}}
}

func (LinkCardLedger) Fields() []ent.Field {
	decimal := map[string]string{dialect.Postgres: "decimal(20,10)"}
	return []ent.Field{
		field.Int64("operation_id").Optional().Nillable(),
		field.Int64("api_key_id"),
		field.Int64("creator_user_id"),
		field.String("entry_type").MaxLen(32),
		field.Float("reserve_delta").SchemaType(decimal),
		field.Float("creator_balance_delta").SchemaType(decimal).Default(0),
		field.Float("quota_before").SchemaType(decimal),
		field.Float("quota_after").SchemaType(decimal),
		field.Float("quota_used_before").SchemaType(decimal),
		field.Float("quota_used_after").SchemaType(decimal),
		field.String("request_id").MaxLen(128).Optional().Nillable(),
		field.Int64("actor_user_id").Optional().Nillable(),
		field.String("reason").MaxLen(500).Default(""),
		field.JSON("metadata", map[string]any{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (LinkCardLedger) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("creator_user_id", "created_at"),
		index.Fields("api_key_id", "created_at"),
	}
}
