package version

import "testing"

func TestVersionSet(t *testing.T) {
	if Version != "v0.1.3alpha" {
		t.Fatalf("Version=%q want v0.1.3alpha", Version)
	}
}
