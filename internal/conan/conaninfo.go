package conan

import (
	"bufio"
	"bytes"
	"strings"
)

// Conaninfo holds the parts of conaninfo.txt shelf uses.
type Conaninfo struct {
	Settings map[string]string
	Options  map[string]string
}

// ParseConaninfo parses a Conan 2 conaninfo.txt. Unknown sections are
// ignored; malformed lines are skipped.
func ParseConaninfo(data []byte) *Conaninfo {
	sections := parseINI(data)
	info := &Conaninfo{Settings: map[string]string{}, Options: map[string]string{}}
	for _, kv := range sections["settings"] {
		info.Settings[kv.key] = kv.value
	}
	for _, kv := range sections["options"] {
		info.Options[kv.key] = kv.value
	}
	return info
}

type keyValue struct{ key, value string }

// parseINI reads "[section]" headers and "key=value" lines. Comments start
// with '#' or ';'. Lines without '=' are returned with an empty value so
// callers can see directives such as include(...).
func parseINI(data []byte) map[string][]keyValue {
	out := map[string][]keyValue{}
	section := ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		out[section] = append(out[section], keyValue{strings.TrimSpace(k), strings.TrimSpace(v)})
	}
	return out
}
