//go:build !windows

package exec

import "context"

// displayMessageWindows is only used on Windows.
func displayMessageWindows(context.Context, string, string) error { return nil }
