// Package migrations embeds the SQL schema migrations applied at startup.
package migrations

import (
	"context"
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var fs embed.FS

// Up applies all pending migrations.
func Up(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(fs)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, ".")
}
