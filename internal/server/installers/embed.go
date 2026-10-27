// Package installers embeds the one-line agent install scripts served at
// /install.sh and /install.ps1.
package installers

import "embed"

//go:embed install.sh install.ps1
var FS embed.FS
