package version

import "testing"

func TestVersionSet(t *testing.T) {
	if Version != "v0.11alpha" {
		t.Fatalf("Version=%q want v0.11alpha", Version)
	}
}
