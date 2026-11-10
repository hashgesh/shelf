// Package i18n translates user-facing text. English is the source language;
// other languages map the English text to a translation.
package i18n

import "strings"

// EnvLang forces the interface language ("en" or "tr").
const EnvLang = "SHELF_LANG"

var lang = "en"

// Set selects the interface language; anything but "tr..." means English.
func Set(l string) {
	if strings.HasPrefix(strings.ToLower(l), "tr") {
		lang = "tr"
	} else {
		lang = "en"
	}
}

// Lang returns the active language code.
func Lang() string { return lang }

// Detect picks the language from SHELF_LANG, the POSIX locale variables,
// then the OS user interface language.
func Detect(lookupEnv func(string) (string, bool)) {
	for _, k := range []string{EnvLang, "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v, ok := lookupEnv(k); ok && v != "" && v != "C" && v != "POSIX" {
			Set(v)
			return
		}
	}
	Set(osLang())
}

// T returns s in the active language; s is the English text.
func T(s string) string {
	if lang == "tr" {
		if t, ok := tr[s]; ok {
			return t
		}
	}
	return s
}

// Error is a sentinel error whose message is translated when printed,
// so it can be declared before the language is chosen.
type Error string

func (e Error) Error() string { return T(string(e)) }
