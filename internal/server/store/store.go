// Package store is the Postgres data access layer. Every tenant-scoped method
// takes the caller's orgID and filters on it; org IDs must come from the
// authenticated session or device credential, never from request input.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("already exists")
)

type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

// querier is satisfied by both the pool and a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// InTx runs fn inside a transaction, committing on success.
func (s *Store) InTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.Pool, fn)
}

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrConflict
		case "22P02": // invalid uuid text: treat a malformed ID as not found
			return ErrNotFound
		case "23503": // FK violation, e.g. group_id from another org or deleted
			return ErrNotFound
		}
	}
	return err
}

func expectOne(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return mapErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// collect scans every row with scan, mapping errors and returning a
// non-nil slice so JSON encodes [] rather than null.
func collect[T any](rows pgx.Rows, err error, scan func(pgx.Rows) (T, error)) ([]T, error) {
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, mapErr(rows.Err())
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
