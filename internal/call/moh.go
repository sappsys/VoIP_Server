package call

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MOHTracks returns .wav files in dir sorted alphanumerically by filename.
func MOHTracks(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".wav") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	tracks := make([]string, len(names))
	for i, n := range names {
		tracks[i] = filepath.Join(dir, n)
	}
	return tracks, nil
}
