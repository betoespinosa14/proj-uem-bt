package compliance

import (
	"regexp"
	"strings"

	"github.com/uem/uem/internal/server/store"
)

// Software flags assigned by Classify.
const (
	FlagGambling = "gambling"
	FlagGame     = "game"
)

// gamblingName matches betting, casino and lottery apps by name. Terms are
// word-bounded so "bet" doesn't hit "Alphabet" or "beta".
var gamblingName = regexp.MustCompile(`(?i)\b(` +
	`casino|poker|slots?|roulette|blackjack|baccarat|sportsbook|betting|bingo|lottery|sabong|e-sabong|` +
	`pokerstars|ggpoker|partypoker|wsop|888(poker|casino|sport)?|bet365|betfair|betway|bwin|unibet|` +
	`draftkings|fanduel|betmgm|caesars sportsbook|william ?hill|ladbrokes|paddy ?power|` +
	`sportsbet|pointsbet|okbet|bingoplus|arenaplus|1xbet|melbet|22bet` +
	`)\b`)

var gamblingPublisher = regexp.MustCompile(`(?i)\b(pokerstars|flutter|entain|playtika|ggpoker|draftkings|fanduel|bet365|sportsbet|digiplus)\b`)

// gameName matches game launchers, popular titles and Linux desktop games.
var gameName = regexp.MustCompile(`(?i)\b(` +
	`steam|epic games( launcher)?|battle\.net|ea app|ubisoft connect|uplay|gog galaxy|riot client|` +
	`rockstar games launcher|xbox|playnite|curseforge|overwolf|` +
	`league of legends|valorant|fortnite|minecraft|roblox|genshin impact|honkai|counter-strike|dota 2|` +
	`world of warcraft|overwatch|call of duty|apex legends|pubg|grand theft auto|gta v|the sims|` +
	`mobile legends|clash of clans|candy crush|solitaire|minesweeper|mahjong|` +
	`aisleriot|gnome-(mines|sudoku|mahjongg|chess|2048)|supertuxkart|0ad|openttd|wesnoth` +
	`)\b`)

var gamePublisher = regexp.MustCompile(`(?i)\b(` +
	`valve|epic games|riot games|blizzard|electronic arts|ubisoft|activision|rockstar games|bethesda|` +
	`cd projekt|square enix|bandai namco|capcom|sega|krafton|garena|moonton|supercell|king\.com|` +
	`mihoyo|hoyoverse|cognosphere|roblox|mojang|zynga|gameloft|netease games|tencent games` +
	`)\b`)

// vmShortcut matches the version Parallels gives the Mac shortcuts it makes
// for apps inside a Windows VM ("Xbox Game Bar", version "Windows 11").
var vmShortcut = regexp.MustCompile(`^Windows \d+$`)

// gameTool matches game-engine tools that game publishers ship but that
// aren't games (Epic's Unreal Engine helpers).
var gameTool = regexp.MustCompile(`(?i)\bunreal\b|^UE\d`)

// antiCheat matches games' anti-cheat drivers. They often stay installed after
// the game itself is removed, so findings say what they are.
var antiCheat = regexp.MustCompile(`(?i)\b(vanguard|easy ?anti-?cheat|battleye|faceit|ricochet|anti-?cheat)\b`)

// IsAntiCheat reports whether an app name is a game's anti-cheat component.
func IsAntiCheat(name string) bool { return antiCheat.MatchString(name) }

// Classify returns FlagGambling, FlagGame or "" for an installed app. Apps
// shipped by the OS vendor (e.g. macOS Chess) are never flagged, nor are
// Parallels shortcuts to apps inside a Windows VM.
func Classify(s store.Software) string {
	if s.Source == "apple" {
		return ""
	}
	if s.Source == "unknown" && vmShortcut.MatchString(s.Version) {
		return ""
	}
	if s.Source == "msstore" && microsoft.MatchString(s.Publisher) {
		return ""
	}
	if gamblingName.MatchString(s.Name) || gamblingPublisher.MatchString(s.Publisher) {
		return FlagGambling
	}
	if gameName.MatchString(s.Name) {
		return FlagGame
	}
	if gamePublisher.MatchString(s.Publisher) && !gameTool.MatchString(s.Name) {
		return FlagGame
	}
	return ""
}

// Software categories assigned by Category.
const (
	CategorySystem     = "system"
	CategoryThirdParty = "third_party"
)

// Microsoft products that ship separately from Windows itself.
var msAddon = regexp.MustCompile(`(?i)\b(365|office|teams|word|excel|powerpoint|outlook|onenote|publisher|access|visio|project|visual studio|sql server|skype)\b`)

// Microsoft's own Store apps, like Macs' built-in apps, come with Windows:
// the Xbox app and Game Bar, and games such as "Solitaire & Casual Games",
// Minesweeper and Mahjong. They aren't games someone chose to install.

// distro matches Linux distribution maintainers/vendors whose packages make up the base OS.
var distro = regexp.MustCompile(`(?i)ubuntu|canonical|debian|red ?hat|fedora|centos|rocky|almalinux|suse|oracle linux|amazon linux`)

var microsoft = regexp.MustCompile(`(?i)^microsoft\b`)

// Hardware drivers on Windows (audio, network, card readers, fingerprint
// readers, database drivers) ship with the machine or another product; they
// aren't apps anyone licenses or uses. Realtek lists some without "Driver"
// in the name ("Realtek Card Reader").
var driver = regexp.MustCompile(`(?i)\bdrivers?\b`)
var hardwareVendor = regexp.MustCompile(`(?i)^realtek\b`)

// component matches helper pieces that come with an app or the OS but that
// no one opens or licenses on their own: updaters, notifiers, crash
// reporters, installers, runtimes and redistributables, licensing helpers,
// and items named as a background service or agent. Adobe ships several
// under internal names (AAM*, ACR_*, AASI*).
var component = regexp.MustCompile(`(?i)` +
	`updaters?\b|\bauto ?update\b|\bupdate (helper|service|notifier)\b|\bnotifier\b|\bnotification client\b|` +
	`\bcrash ?(reporter|processor|handler|pad)\b|\bdiagnostics?\b|installer(agent)?\b|` +
	`\bredistributables?\b|\bruntime\b|\bwebview2\b|` +
	`\blicens(e|ing) ?helper\b|lic(app|helper|util)\b|\bgenuine service\b|clean ?up ?utility\b|` +
	`\bmaintenance service\b|\bhelper\b|\b(service|agent)$|plugin ?host\b|` +
	`^(aam|acr[_ ]|aasi)`)

// IsComponent reports whether an installed app is a helper component rather
// than an app someone would open: "not used" is meaningless for those, so
// they're left out of license reviews.
//
// Underscores count as spaces, so "adobe_licensing_helper" reads as
// "adobe licensing helper" (an underscore is otherwise part of a word).
func IsComponent(s store.Software) bool {
	return component.MatchString(strings.ReplaceAll(s.Name, "_", " "))
}

// winUpdate matches Windows update packages listed as installed software,
// e.g. "Windows Setup Remediations (x64) (KB4023057)".
var winUpdate = regexp.MustCompile(`\bKB\d{6,8}\b`)

// Category splits apps into what ships with or comes from the OS vendor
// (CategorySystem) and everything installed on top (CategoryThirdParty),
// using only the source and publisher the agent reports.
func Category(s store.Software) string {
	switch s.Source {
	case "apple": // Apple-signed (macOS built-ins)
		return CategorySystem
	case "registry", "msstore": // msstore: Microsoft Store apps (Windows)
		if winUpdate.MatchString(s.Name) {
			return CategorySystem // a Windows update, often listed without a publisher
		}
		if driver.MatchString(s.Name) || hardwareVendor.MatchString(s.Publisher) {
			return CategorySystem
		}
		if microsoft.MatchString(s.Publisher) && !msAddon.MatchString(s.Name) {
			return CategorySystem
		}
	case "dpkg", "rpm", "snap":
		if distro.MatchString(s.Publisher) {
			return CategorySystem
		}
	case "unknown":
		// Parallels' Mac shortcuts to the apps inside a Windows VM
		// (Calculator, Command Prompt, version "Windows 11") are Windows
		// built-ins, not software installed on the Mac.
		if vmShortcut.MatchString(s.Version) {
			return CategorySystem
		}
	}
	return CategoryThirdParty // mac_app_store, identified_developer, unknown
}

// flagged returns the names of installed apps carrying the given flag, each
// name once (two installed versions of one app are one finding).
func flagged(f Facts, flag string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range f.Software {
		if Classify(s) == flag && !seen[s.Name] {
			seen[s.Name] = true
			out = append(out, s.Name)
		}
	}
	return out
}
