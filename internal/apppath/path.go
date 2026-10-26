// Package apppath keeps control and engine path identity in agreement.
package apppath

import (
	"github.com/AyoubAchour/net-switch/internal/jsvalue"
	"regexp"
	"strings"
)

const version = `[0-9]+(?:\.[0-9]+)+`

var folder = regexp.MustCompile(`(?i)^(app-|v)?(` + version + `)$`)
var store = regexp.MustCompile(`(?i)^([^_]+)_(` + version + `)(_(?:x64|x86|arm64|arm|neutral)__[^_]+)$`)

type Rules struct {
	Separator, RegexFlags, VersionPattern string
	FoldCase                              func(string) string
	VersionFolder                         func(string) (before, after string, ok bool)
}

var Windows = Rules{
	Separator: `\`, RegexFlags: `(?i)`, VersionPattern: version,
	FoldCase: jsvalue.Lower,
	VersionFolder: func(s string) (string, string, bool) {
		if m := folder.FindStringSubmatch(s); m != nil {
			return m[1], "", true
		}
		if m := store.FindStringSubmatch(s); m != nil {
			return m[1] + "_", m[3], true
		}
		return "", "", false
	},
}

// Linux paths are case-sensitive. Package identities (Flatpak, Snap and
// registered AppImage names) take precedence over this path fallback.
var Linux = Rules{
	Separator: "/", VersionPattern: version,
	FoldCase: func(s string) string { return s },
	VersionFolder: func(s string) (string, string, bool) {
		if m := folder.FindStringSubmatch(s); m != nil {
			return m[1], "", true
		}
		return "", "", false
	},
}

func parts(s string, rules Rules, pattern bool) string {
	segments := strings.Split(s, rules.Separator)
	for i, segment := range segments {
		before, after, variable := segment, "", false
		if i < len(segments)-1 {
			if a, b, ok := rules.VersionFolder(segment); ok {
				before, after, variable = a, b, true
			}
		}
		middle := ""
		if variable {
			middle = "*"
			if pattern {
				middle = rules.VersionPattern
			}
		}
		if pattern {
			before, after = regexp.QuoteMeta(before), regexp.QuoteMeta(after)
		}
		segments[i] = before + middle + after
	}
	separator := rules.Separator
	if pattern {
		separator = regexp.QuoteMeta(separator)
	}
	return strings.Join(segments, separator)
}

func Normalize(s string, rules Rules) string { return rules.FoldCase(parts(s, rules, false)) }
func Pattern(s string, rules Rules) string {
	return rules.RegexFlags + "^" + parts(s, rules, true) + "$"
}

// Unknown is the identity of traffic no app owns: connections whose owner
// could not be found, and traffic Windows forwards for virtual machines, WSL
// and containers, which has no process on this PC. Real identities are
// absolute paths or prefixed sandbox ids, so this name never collides with
// one; a pin stored under it places that whole bucket on a line.
const Unknown = "unknown"

func Key(s string) string {
	if s == "" {
		return Unknown
	}
	return Normalize(s, Windows)
}
