package compliance

import (
	"regexp"

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

// Classify returns FlagGambling, FlagGame or "" for an installed app. Apps
// shipped by the OS vendor (e.g. macOS Chess) are never flagged.
func Classify(s store.Software) string {
	if s.Source == "apple" {
		return ""
	}
	if gamblingName.MatchString(s.Name) || gamblingPublisher.MatchString(s.Publisher) {
		return FlagGambling
	}
	if gameName.MatchString(s.Name) || gamePublisher.MatchString(s.Publisher) {
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
var msAddon = regexp.MustCompile(`(?i)\b(365|office|teams|access|visio|project|visual studio|sql server|skype)\b`)

// distro matches Linux distribution maintainers/vendors whose packages make up the base OS.
var distro = regexp.MustCompile(`(?i)ubuntu|canonical|debian|red ?hat|fedora|centos|rocky|almalinux|suse|oracle linux|amazon linux`)

var microsoft = regexp.MustCompile(`(?i)^microsoft\b`)

// Category splits apps into what ships with or comes from the OS vendor
// (CategorySystem) and everything installed on top (CategoryThirdParty),
// using only the source and publisher the agent reports.
func Category(s store.Software) string {
	switch s.Source {
	case "apple": // Apple-signed (macOS built-ins)
		return CategorySystem
	case "registry":
		if microsoft.MatchString(s.Publisher) && !msAddon.MatchString(s.Name) {
			return CategorySystem
		}
	case "dpkg", "rpm", "snap":
		if distro.MatchString(s.Publisher) {
			return CategorySystem
		}
	}
	return CategoryThirdParty // mac_app_store, identified_developer, unknown
}

// flagged returns the names of installed apps carrying the given flag.
func flagged(f Facts, flag string) []string {
	out := []string{}
	for _, s := range f.Software {
		if Classify(s) == flag {
			out = append(out, s.Name)
		}
	}
	return out
}
