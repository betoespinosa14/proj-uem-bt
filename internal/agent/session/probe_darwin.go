package session

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

const supported = true

func probe(ctx context.Context) (Sample, error) {
	idle, err := hidIdle(ctx)
	if err != nil {
		return Sample{}, err
	}
	s := Sample{IdleMS: idle.Milliseconds()}
	if out, err := run(ctx, "/usr/bin/lsappinfo", "front"); err == nil {
		if asn := parseFront(out); asn != "" {
			if info, err := run(ctx, "/usr/bin/lsappinfo", "info", "-only", "bundlepath", asn); err == nil {
				s.AppPath = parseBundlePath(info)
			}
		}
	}
	return s, nil
}

func hidIdle(ctx context.Context) (time.Duration, error) {
	out, err := run(ctx, "/usr/sbin/ioreg", "-c", "IOHIDSystem", "-d", "4", "-r", "-k", "HIDIdleTime")
	if err != nil {
		return 0, err
	}
	return parseHIDIdle(out)
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

var (
	asnRe = regexp.MustCompile(`ASN:0x[0-9a-fA-F]+-0x[0-9a-fA-F]+:`)
	// macOS 14+ prints `bundle path="..."`; macOS 13 prints `"LSBundlePath"="..."`.
	bundlePathRe = regexp.MustCompile(`(?:bundle path|"LSBundlePath")="([^"]+)"`)
	hidIdleRe    = regexp.MustCompile(`"HIDIdleTime" = (\d+)`)
)

// parseFront extracts the application serial number from `lsappinfo front`.
func parseFront(out string) string { return asnRe.FindString(out) }

// parseBundlePath extracts the bundle path from `lsappinfo info`.
func parseBundlePath(out string) string {
	if m := bundlePathRe.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

// parseHIDIdle reads HIDIdleTime (nanoseconds since the last input) from ioreg.
func parseHIDIdle(out string) (time.Duration, error) {
	m := hidIdleRe.FindStringSubmatch(out)
	if m == nil {
		return 0, strconv.ErrSyntax
	}
	ns, err := strconv.ParseInt(m[1], 10, 64)
	return time.Duration(ns), err
}
