package postgres

import "testing"

func TestValidUUID(t *testing.T) {
	if !validUUID("3f2b8c1e-1a2b-4c3d-8e9f-0123456789ab") {
		t.Fatal("valid rejected")
	}
	for _, s := range []string{"", "abc", "3f2b8c1e-1a2b-4c3d-8e9f-0123456789aZ", "3f2b8c1e11a2b-4c3d-8e9f-0123456789ab"} {
		if validUUID(s) {
			t.Fatalf("invalid accepted %q", s)
		}
	}
	if escapeLike(`50%_\`) != `50\%\_\\` {
		t.Fatal("escapeLike")
	}
}
