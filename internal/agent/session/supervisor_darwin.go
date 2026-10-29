package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// listSessions returns the console user: the one whose GUI session is on
// screen. Fast-user-switched sessions in the background aren't active.
func listSessions(context.Context) ([]userSession, error) {
	fi, err := os.Stat("/dev/console")
	if err != nil {
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid == 0 { // root owns the console at the login window
		return nil, nil
	}
	u, err := user.LookupId(strconv.Itoa(int(st.Uid)))
	if err != nil {
		return nil, err
	}
	return []userSession{{key: u.Uid, user: u.Username, id: st.Uid}}, nil
}

// startHelper runs the probe in the user's GUI bootstrap context, as that
// user, so lsappinfo sees their front app.
func startHelper(_ context.Context, exe string, us userSession) (*helperProc, error) {
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("the agent must run as root to start session probes")
	}
	cmd := exec.Command("/bin/launchctl", "asuser", strconv.Itoa(int(us.id)),
		"/usr/bin/sudo", "-n", "-u", us.user, exe, "session-probe")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	return &helperProc{
		stdout: stdout,
		stop:   stopper(stdin, done, func() { _ = cmd.Process.Kill() }),
		wait: func() error {
			defer close(done)
			return cmd.Wait()
		},
	}, nil
}
