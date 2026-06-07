package database

import (
	"context"
	"database/sql"
	"fmt"
	"stella/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/vksir/vkiss-lib/pkg/log"
	"github.com/vksir/vkiss-lib/pkg/util/errutil"
	_ "modernc.org/sqlite"
)

var G *ent.Client

func New(path ...string) (*ent.Client, error) {
	dsn := "file:ent?mode=memory&cache=shared&_pragma=foreign_keys(1)"
	if len(path) == 1 && path[0] != "" {
		dsn = fmt.Sprintf("file:%s?cache=shared&_pragma=foreign_keys(1)", path[0])
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errutil.Wrap(err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	return client, nil
}

func WithTx(ctx context.Context, client *ent.Client, fn func(ctx context.Context, tx *ent.Tx) error) error {
	tx, exists := ctx.Value("tx").(*ent.Tx)
	if !exists {
		var err error
		tx, err = client.Tx(ctx)
		if err != nil {
			return errutil.Wrap(err)
		}
		ctx = context.WithValue(ctx, "tx", tx)

		defer func() {
			if v := recover(); v != nil {
				log.DebugC(ctx, "begin tx recover rollback")
				err = tx.Rollback()
				if err != nil {
					log.ErrorC(ctx, "rollback transaction failed", "err", err)
				}
				panic(v)
			}
		}()
	}

	err := fn(ctx, tx)

	if !exists {
		if err != nil {
			log.DebugC(ctx, "begin tx rollback")
			if err := tx.Rollback(); err != nil {
				log.ErrorC(ctx, "rollback transaction failed", "err", err)
			}
			return err
		}

		log.DebugC(ctx, "begin tx commit")
		err = tx.Commit()
		if err != nil {
			return errutil.Wrap(err)
		}
	}
	return nil
}

func Init(path string) {
	c, err := New(path)
	errutil.Check(err)
	G = c

	err = c.Schema.Create(context.Background())
	errutil.Check(err)
}
