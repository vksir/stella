// Package database 负责 SQLite 数据库的打开与访问。
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // 注册 sqlite 驱动

	"github.com/vksir/stella/internal/config"
)

// Open 打开 SQLite 数据库，并校验连接可用。
func Open(cfg config.DatabaseConfig) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// busy_timeout 避免并发写锁报错，foreign_keys 开启外键约束
	dsn := "file:" + filepath.ToSlash(cfg.Path) + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}
