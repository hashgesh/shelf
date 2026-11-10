package conan

import (
	"sort"
	"strings"
)

// InstallCommand builds a `conan install` command line that resolves to b:
// the recipe revision is pinned, the matched profile is used as host
// profile (or every known setting is passed with -s when none matched), and
// the binary's options are passed with -o. See quote for the quoting rules.
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

// quote makes s one literal shell argument. No single quoting style is
// literal in bash, cmd and PowerShell at once, so:
//   - plain arguments are left alone;
//   - arguments with ordinary metacharacters (space, *, #, ...) are
//     double-quoted, which all three shells take literally;
//   - arguments with characters that stay special inside double quotes
//     ($, `, ", \, !) are single-quoted, which bash and PowerShell take
//     literally (cmd has no literal quoting for these). An embedded single
//     quote is doubled, the PowerShell form; such values are not expected in
//     Conan references, settings or options.
func quote(s string) string {
	switch {
	case strings.ContainsAny(s, "$`\"\\!"):
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	case strings.ContainsAny(s, " \t*?#&|<>;()'^%~[]{}"):
		return `"` + s + `"`
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
