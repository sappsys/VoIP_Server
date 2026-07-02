package call

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MOHTracks returns .wav files in dir sorted alphanumerically by filename.
// If dir is empty, falls back to moh.wav in the parent directory (legacy layout).
func MOHTracks(dir string) ([]string, error) {
	tracks, err := wavFilesIn(dir)
	if err != nil {
		return nil, err
	}
	if len(tracks) > 0 {
		return tracks, nil
	}
	parent := filepath.Dir(dir)
	for _, name := range []string{"moh.wav", "hold.wav"} {
		p := filepath.Join(parent, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return []string{p}, nil
		}
	}
	return nil, fmt.Errorf("no moh wav files in %s", dir)
}

func wavFilesIn(dir string) ([]string, error) {
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
