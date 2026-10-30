package config

import "encoding/json"

const redacted = "********"

// Secret holds a credential. Every formatting path (fmt verbs, JSON) prints a
// redacted placeholder; only Reveal returns the real value.
type Secret string

// Reveal returns the raw secret. Use it only to build request headers or to
// write the config file.
func (s Secret) Reveal() string { return string(s) }

// IsSet reports whether the secret is non-empty.
func (s Secret) IsSet() bool { return s != "" }

// String implements fmt.Stringer.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return redacted
}

// GoString implements fmt.GoStringer so %#v is redacted too.
func (s Secret) GoString() string { return `"` + s.String() + `"` }

// MarshalJSON implements json.Marshaler.
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// MarshalText implements encoding.TextMarshaler (used by YAML and others).
func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }
