package zen

import (
	"regexp"
	"strconv"
	"testing"
)

// The gate accepts ids of the exact shape "ses_" + 12 lowercase hex + 14 base62 chars.
var sessionShape = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

func TestSessionIDShape(t *testing.T) {
	for i := 0; i < 50; i++ {
		id, err := SessionID(uint64(i))
		if err != nil {
			t.Fatalf("SessionID: %v", err)
		}
		if !sessionShape.MatchString(id) {
			t.Fatalf("session id %q does not match the required shape", id)
		}
		if len(id) != 30 {
			t.Fatalf("session id %q has length %d, want 30", id, len(id))
		}
	}
}

// The low 12 bits of the hex field must decode to 0xfff-counter with a counter <= 63.
func TestSessionIDCounterEncoding(t *testing.T) {
	for _, counter := range []uint64{0, 1, 30, 62, 63} {
		id, err := SessionID(counter)
		if err != nil {
			t.Fatalf("SessionID: %v", err)
		}
		low, err := strconv.ParseUint(id[13:16], 16, 16) // last 3 hex chars = low 12 bits
		if err != nil {
			t.Fatalf("parse low bits: %v", err)
		}
		want := 0xfff - counter
		if low != want {
			t.Fatalf("counter %d: low bits = 0x%x, want 0x%x", counter, low, want)
		}
	}
}

// A counter above the gate's limit must be clamped, never emitted as-is.
func TestSessionIDClampsCounter(t *testing.T) {
	id, err := SessionID(1000)
	if err != nil {
		t.Fatalf("SessionID: %v", err)
	}
	low, _ := strconv.ParseUint(id[13:16], 16, 16)
	if low != 0xfff-63 {
		t.Fatalf("over-limit counter not clamped: low bits = 0x%x", low)
	}
}

func TestRequiredTools(t *testing.T) {
	want := map[string]bool{"bash": true, "glob": true, "grep": true, "read": true}
	if len(RequiredToolNames) != len(want) {
		t.Fatalf("RequiredToolNames = %v, want %d entries", RequiredToolNames, len(want))
	}
	for _, name := range RequiredToolNames {
		if !want[name] {
			t.Fatalf("unexpected required tool %q", name)
		}
	}
}
