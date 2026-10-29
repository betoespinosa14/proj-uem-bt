package session

import (
	"context"
	"os"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// listSessions returns every active session with a logged-on user: the
// console and any Remote Desktop sessions.
func listSessions(context.Context) ([]userSession, error) {
	var info *windows.WTS_SESSION_INFO
	var n uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &info, &n); err != nil {
		return nil, err
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(info)))
	var out []userSession
	for _, s := range unsafe.Slice(info, n) {
		if s.State != windows.WTSActive || s.SessionID == 0 {
			continue
		}
		var tok windows.Token
		if windows.WTSQueryUserToken(s.SessionID, &tok) != nil {
			continue // no user logged on
		}
		name, err := tokenUser(tok)
		tok.Close()
		if err != nil {
			continue
		}
		out = append(out, userSession{key: name + "@" + strconv.FormatUint(uint64(s.SessionID), 10), user: name, id: s.SessionID})
	}
	return out, nil
}

// tokenUser returns DOMAIN\user, the same form the process list reports.
func tokenUser(tok windows.Token) (string, error) {
	tu, err := tok.GetTokenUser()
	if err != nil {
		return "", err
	}
	account, domain, _, err := tu.User.Sid.LookupAccount("")
	if err != nil {
		return "", err
	}
	return domain + `\` + account, nil
}

// startHelper launches the probe inside the user's session, on their
// interactive desktop, with its stdin and stdout connected to private pipes.
func startHelper(_ context.Context, exe string, us userSession) (*helperProc, error) {
	var tok windows.Token
	if err := windows.WTSQueryUserToken(us.id, &tok); err != nil {
		return nil, err
	}
	defer tok.Close()

	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, sa, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outR, &outW, sa, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, err
	}
	// Only the probe's ends are inherited.
	_ = windows.SetHandleInformation(inW, windows.HANDLE_FLAG_INHERIT, 0)
	_ = windows.SetHandleInformation(outR, windows.HANDLE_FLAG_INHERIT, 0)

	var env *uint16
	if err := windows.CreateEnvironmentBlock(&env, tok, false); err != nil {
		env = nil
	} else {
		defer windows.DestroyEnvironmentBlock(env)
	}
	desktop, _ := windows.UTF16PtrFromString(`winsta0\default`)
	cmdline, _ := windows.UTF16PtrFromString(syscall.EscapeArg(exe) + " session-probe")
	si := windows.StartupInfo{
		Cb:         uint32(unsafe.Sizeof(windows.StartupInfo{})),
		Desktop:    desktop,
		Flags:      windows.STARTF_USESTDHANDLES | windows.STARTF_USESHOWWINDOW,
		ShowWindow: windows.SW_HIDE,
		StdInput:   inR,
		StdOutput:  outW,
		StdErr:     outW,
	}
	var pi windows.ProcessInformation
	err := windows.CreateProcessAsUser(tok, nil, cmdline, nil, nil, true,
		windows.CREATE_NO_WINDOW|windows.CREATE_UNICODE_ENVIRONMENT, env, nil, &si, &pi)
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, err
	}
	windows.CloseHandle(pi.Thread)
	stdin := os.NewFile(uintptr(inW), "probe-stdin")
	stdout := os.NewFile(uintptr(outR), "probe-stdout")
	done := make(chan struct{})
	return &helperProc{
		stdout: stdout,
		stop:   stopper(stdin, done, func() { _ = windows.TerminateProcess(pi.Process, 1) }),
		wait: func() error {
			defer close(done)
			defer windows.CloseHandle(pi.Process)
			defer stdout.Close()
			_, err := windows.WaitForSingleObject(pi.Process, windows.INFINITE)
			return err
		},
	}, nil
}
