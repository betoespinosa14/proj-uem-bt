// Package session measures active application use. The agent service can't
// see a user's front window or input idle time, so it runs a small probe
// (uem-agent session-probe) inside each interactive user session and reads
// its samples over a private stdout pipe.
package session

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// ProbeInterval is how often the probe samples the front app and idle time.
const ProbeInterval = 5 * time.Second

// Sample is one probe observation.
type Sample struct {
	AppPath string `json:"app_path"` // macOS: front app bundle path; Windows: front window's executable
	IdleMS  int64  `json:"idle_ms"`  // time since the last keyboard or mouse input
}

// ErrUnsupported is returned on platforms without a probe.
var ErrUnsupported = errors.New("active usage probing is not supported on this platform")

// RunProbe writes a Sample as a JSON line to out every ProbeInterval until
// in is closed (the agent closes it to stop the probe, and it closes by
// itself when the agent exits).
func RunProbe(in io.Reader, out io.Writer) error {
	if !supported {
		return ErrUnsupported
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, _ = io.Copy(io.Discard, in)
		cancel()
	}()
	enc := json.NewEncoder(out)
	t := time.NewTicker(ProbeInterval)
	defer t.Stop()
	for {
		s, err := probe(ctx)
		if err == nil {
			if err := enc.Encode(s); err != nil {
				return err // the agent went away
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// ReadSamples decodes probe output, skipping lines that aren't samples.
func ReadSamples(r io.Reader, fn func(Sample)) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var s Sample
		if json.Unmarshal(sc.Bytes(), &s) == nil {
			fn(s)
		}
	}
	return sc.Err()
}

// Stdio is the probe's standard input and output for the session-probe
// subcommand.
func Stdio() (io.Reader, io.Writer) { return os.Stdin, os.Stdout }
