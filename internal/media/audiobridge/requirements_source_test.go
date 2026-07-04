package audiobridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// REQ-BRIDGE-4: bridging must never call ClockDisable — diago Write() panics on nil ticker.
func TestREQ_BRIDGE_NoClockDisableInPackage(t *testing.T) {
	root := filepath.Join("..", "audiobridge")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), ".ClockDisable()") {
			t.Errorf("REQ-BRIDGE-4: %s must not call ClockDisable", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
