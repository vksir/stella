// ent/schema/message.go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Message struct {
	ent.Schema
}

func (Message) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_id"),
		field.String("user_id").Optional().Nillable(),
		field.String("role"),
		field.String("type"),
		field.Text("content"),
		field.Text("tool_calls").Default(""),
		field.String("tool_id").Default(""),
		field.Bytes("data").Optional().Nillable(),
		field.Time("created_at").Default(time.Now),
	}
}

func (Message) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("session", Session.Type).Ref("messages").Unique().Field("session_id").Required(),
	}
}
