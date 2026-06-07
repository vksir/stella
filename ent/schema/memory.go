package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Memory struct {
	ent.Schema
}

func (Memory) Fields() []ent.Field {
	return []ent.Field{
		field.String("user_id"),
		field.String("content"),
		field.JSON("vector", []float32{}),
		field.Time("last_accessed_at").Default(time.Now).UpdateDefault(time.Now),
		field.Time("created_at").Default(time.Now),
	}
}

func (Memory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("memories").Unique().Field("user_id").Required(),
	}
}
