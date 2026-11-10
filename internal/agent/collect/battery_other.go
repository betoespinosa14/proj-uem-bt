//go:build !darwin

package collect

import (
	"strings"

	"github.com/distatus/battery"

	"github.com/uem/uem/internal/protocol"
)

// batteryInfo reads the first battery through the battery library (Windows, Linux).
// A partial read (some field unavailable) still counts as long as the charge is known.
func batteryInfo() protocol.Battery {
	bats, _ := battery.GetAll() // on a partial error the batteries are still returned
	if len(bats) == 0 || bats[0] == nil || bats[0].Full == 0 {
		return protocol.Battery{Present: false, State: "none"}
	}
	b := bats[0]
	// Windows can report a fully charged battery slightly above its full capacity (e.g. 101%).
	pct := min(b.Current/b.Full*100, 100)
	out := protocol.Battery{Present: true, Percent: round1(pct), State: strings.ToLower(b.State.String())}
	if b.Design > 0 {
		h := round1(b.Full / b.Design * 100)
		out.HealthPct = &h
	}
	return out
}
