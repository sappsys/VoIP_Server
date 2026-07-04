package conference

import (
	"os"
	"strings"
	"testing"
)

func readConferenceCollectPINSource(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("conference.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func containsAll(body string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(body, p) {
			return false
		}
	}
	return true
}
