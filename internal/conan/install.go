package conan

import (
	"sort"
	"strings"
)

// InstallCommand builds a `conan install` command line that resolves to b:
// the recipe revision is pinned, the matched profile is used as host
// profile (or every known setting is passed with -s when none matched), and
// the binary's options are passed with -o. Arguments that may contain shell
// metacharacters are double-quoted, which works in bash, cmd and PowerShell.
func InstallCommand(b *Binary) string {
	ref := b.Ref()
	if b.RecipeRevision != "" {
		ref += "#" + b.RecipeRevision
	}
	parts := []string{"conan", "install", "--requires=" + quote(ref)}
	if b.Profile != "" {
		parts = append(parts, "-pr:h", quote(b.Profile))
	} else {
		for _, k := range sortedKeys(b.Settings) {
			parts = append(parts, "-s", quote(k+"="+b.Settings[k]))
		}
	}
	for _, k := range sortedKeys(b.Options) {
		parts = append(parts, "-o", quote(b.Name+"/*:"+k+"="+b.Options[k]))
	}
	return strings.Join(parts, " ")
}

func quote(s string) string {
	if strings.ContainsAny(s, " *?#&|<>;()$`'\"!") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
