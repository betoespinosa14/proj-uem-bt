package conn

import "golang.org/x/sys/windows"

// inServiceSession reports whether a process runs in session 0, where
// Windows runs services and no user signs in.
func inServiceSession(pid int32) bool {
	var session uint32
	if err := windows.ProcessIdToSessionId(uint32(pid), &session); err != nil {
		return false // unknown: keep counting, as before
	}
	return session == 0
}
