//go:build !darwin && !windows

package session

import (
	"context"
	"errors"
)

func listSessions(context.Context) ([]userSession, error) { return nil, nil }

func startHelper(context.Context, string, userSession) (*helperProc, error) {
	return nil, errors.New("unsupported")
}
