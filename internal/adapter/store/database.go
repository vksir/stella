package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	"github.com/vksir/stella/internal/core/agent"
)

// Database 使用 SQLite 保存会话。
type Database struct {
	db *sql.DB
}

var _ agent.Store = (*Database)(nil)

// NewDatabase 初始化会话表，连接由调用方管理。
func NewDatabase(ctx context.Context, db *sql.DB) (*Database, error) {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS agent_sessions (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			data TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS agent_sessions_name ON agent_sessions(name);
	`); err != nil {
		return nil, fmt.Errorf("initialize session store: %w", err)
	}
	return &Database{db: db}, nil
}

func (s *Database) Find(ctx context.Context, id uuid.UUID) (*agent.Session, error) {
	return s.find(ctx, "SELECT data FROM agent_sessions WHERE id = ?", id.String())
}

func (s *Database) FindByName(ctx context.Context, name string) (*agent.Session, error) {
	return s.find(ctx, "SELECT data FROM agent_sessions WHERE name = ? LIMIT 1", name)
}

func (s *Database) find(ctx context.Context, query string, key string) (*agent.Session, error) {
	var data []byte
	if err := s.db.QueryRowContext(ctx, query, key).Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, agent.ErrSessionNotFound
		}
		return nil, fmt.Errorf("find session: %w", err)
	}
	var sess agent.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("decode session: %w", err)
	}
	return &sess, nil
}

func (s *Database) Save(ctx context.Context, sess *agent.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_sessions (id, name, data) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, data = excluded.data
	`, sess.ID.String(), sess.Name, string(data)); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (s *Database) AppendMessage(ctx context.Context, id uuid.UUID, messages ...agent.Message) error {
	if len(messages) == 0 {
		var exists int
		if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM agent_sessions WHERE id = ?", id.String()).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return agent.ErrSessionNotFound
			}
			return fmt.Errorf("find session: %w", err)
		}
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin append messages: %w", err)
	}
	defer tx.Rollback()

	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("encode message: %w", err)
		}
		// 空消息列表统一为数组，追加在数据库内完成。
		result, err := tx.ExecContext(ctx, `
			UPDATE agent_sessions
			SET data = json_insert(
				json_set(data, '$.messages', json(COALESCE(json_extract(data, '$.messages'), '[]'))),
				'$.messages[#]', json(?)
			)
			WHERE id = ?
		`, string(data), id.String())
		if err != nil {
			return fmt.Errorf("append session message: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("count updated sessions: %w", err)
		}
		if affected == 0 {
			return agent.ErrSessionNotFound
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit append messages: %w", err)
	}
	return nil
}

func (s *Database) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM agent_sessions WHERE id = ?", id.String()); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
