package exec

import (
	"context"
	"errors"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procWTSSendMessageW = windows.NewLazySystemDLL("wtsapi32.dll").NewProc("WTSSendMessageW")

// displayMessageWindows shows a message box in every signed-in session with our
// own title, using WTSSendMessageW (the API msg.exe uses). msg.exe titles the box
// "Message from <user> <date time>" and can't take a title, so it's only the
// fallback when this fails.
func displayMessageWindows(ctx context.Context, title, message string) error {
	if err := wtsSendToActiveSessions(title, message); err == nil {
		return nil
	} else if errors.Is(err, errNoSession) {
		return err
	}
	return runCmd(ctx, "msg.exe", "*", "/TIME:600", title+": "+message)
}

var errNoSession = errors.New("no user is signed in")

func wtsSendToActiveSessions(title, message string) error {
	var sessions *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &sessions, &count); err != nil {
		return err
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(sessions)))
	list := unsafe.Slice(sessions, count)

	t, tLen := wtsString(title)
	m, mLen := wtsString(message)
	sent := 0
	var lastErr error
	for _, s := range list {
		if s.State != windows.WTSActive || s.SessionID == 0 {
			continue
		}
		var response uint32
		// Returns at once (bWait = false); the box closes itself after 10 minutes, like msg.exe /TIME:600.
		r, _, err := procWTSSendMessageW.Call(
			0, uintptr(s.SessionID),
			uintptr(unsafe.Pointer(t)), uintptr(tLen),
			uintptr(unsafe.Pointer(m)), uintptr(mLen),
			uintptr(windows.MB_OK|windows.MB_ICONINFORMATION|windows.MB_SETFOREGROUND),
			600, uintptr(unsafe.Pointer(&response)), 0,
		)
		if r == 0 {
			lastErr = err
			continue
		}
		sent++
	}
	if sent > 0 {
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errNoSession
}

// wtsString returns a NUL-terminated UTF-16 copy of s and its length in bytes
// without the terminator, as WTSSendMessageW expects.
func wtsString(s string) (*uint16, int) {
	u := utf16.Encode([]rune(s))
	return &append(u, 0)[0], len(u) * 2
}
