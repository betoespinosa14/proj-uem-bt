package session

import (
	"context"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

const supported = true

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetLastInputInfo = user32.NewProc("GetLastInputInfo")
	procGetTickCount     = kernel32.NewProc("GetTickCount")
)

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

func probe(context.Context) (Sample, error) {
	li := lastInputInfo{cbSize: uint32(unsafe.Sizeof(lastInputInfo{}))}
	if r, _, err := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&li))); r == 0 {
		return Sample{}, err
	}
	now, _, _ := procGetTickCount.Call()
	s := Sample{IdleMS: int64(uint32(now) - li.dwTime)} // uint32 arithmetic survives the 49-day wrap
	s.AppPath, _ = foregroundExe()
	return s, nil
}

func foregroundExe() (string, error) {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return "", errors.New("no foreground window")
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil {
		return "", err
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}
