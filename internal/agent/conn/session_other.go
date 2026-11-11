//go:build !windows

package conn

// inServiceSession is Windows-only; elsewhere service accounts are told
// apart by user ID (see usage.IsHuman).
func inServiceSession(int32) bool { return false }
