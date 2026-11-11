package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/uem/uem/internal/server/audit"
	"github.com/uem/uem/internal/server/store"
)

// resetMFA is the last resort when nobody who could reset a user's two-step
// sign-in from the console can sign in:
//
//	uem-server reset-mfa <email>
//
// It turns two-step sign-in off for that user, signs them out everywhere and
// records it in the audit log. It reads DATABASE_URL like the server.
func resetMFA(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: uem-server reset-mfa <email>")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, env("DATABASE_URL", "postgres://uem:uem@localhost:5432/uem?sslmode=disable"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		return 1
	}
	defer pool.Close()
	st := store.New(pool)
	user, err := st.GetUserByEmail(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "no user with email %q\n", args[0])
		return 1
	}
	if !user.MFAEnabled {
		fmt.Printf("%s doesn't have two-step sign-in on; nothing to do.\n", user.Email)
		return 0
	}
	if err := st.DisableMFA(ctx, user.ID); err != nil {
		fmt.Fprintln(os.Stderr, "reset failed:", err)
		return 1
	}
	_ = st.DeleteUserSessions(ctx, user.ID)
	audit.New(st).Record(ctx, audit.Event{OrgID: user.OrgID,
		Actor:  audit.Actor{Type: "system", Email: "uem-server reset-mfa"},
		Action: "user.mfa_reset", TargetType: "user", TargetID: user.ID,
		Details: map[string]any{"email": user.Email, "via": "command line"}})
	fmt.Printf("Two-step sign-in is now off for %s, and they were signed out. "+
		"They can sign in with their password and turn it on again.\n", user.Email)
	return 0
}
