package mail

import "testing"

func TestMaskEmail(t *testing.T) {
	for in, want := range map[string]string{"berat@example.com": "b***@example.com", "x": "***", "@a.b": "***"} {
		if got := MaskEmail(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
