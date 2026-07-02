package version

import "testing"

func TestVersionSet(t *testing.T) {
	if Version != "v0.10alpha" {
		t.Fatalf("Version=%q want v0.10alpha", Version)
	}
}
