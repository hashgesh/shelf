package format

import (
	"testing"
	"time"
)

func TestSize(t *testing.T) {
	tests := map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 1536: "1.5 KB", 5 << 20: "5.0 MB", 3 << 30: "3.0 GB", 5 << 40: "5120.0 GB"}
	for in, want := range tests {
		if got := Size(in); got != want {
			t.Errorf("Size(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestHelpers(t *testing.T) {
	if Time(time.Time{}) != "-" || Dash("") != "-" || Dash("x") != "x" {
		t.Error("zero handling")
	}
	if ShortID("0123456789") != "01234567" || ShortID("abc") != "abc" {
		t.Error("ShortID")
	}
}
