// internal/memory/history.go
package memory

import (
	"context"
	"stella/ent"
	"stella/ent/message"
	"stella/pkg/database"
)

func GetRecentHistory(ctx context.Context, sessionID string, limit int) ([]*ent.Message, error) {
	return database.G.Message.Query().
		Where(
			message.SessionID(sessionID),
		).
		Order(ent.Desc(message.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
}
