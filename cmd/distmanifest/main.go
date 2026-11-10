// Command distmanifest writes dist/manifest.json (version and sha256 of each agent
// binary), which the server uses to offer agent updates. Run by `make build-agents`.
package main

import (
	"fmt"
	"os"

	"github.com/uem/uem/internal/agentdist"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: distmanifest <dist dir> <version>")
		os.Exit(2)
	}
	m, err := agentdist.Write(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "distmanifest:", err)
		os.Exit(1)
	}
	fmt.Printf("  manifest: %s, %d files\n", m.Version, len(m.Files))
}
