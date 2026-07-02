package call

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMOHTracksAlphanumericOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"10.wav", "02.wav", "01.wav", "notes.txt", "sub"} {
		path := filepath.Join(dir, name)
		if name == "sub" {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tracks, err := MOHTracks(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"01.wav", "02.wav", "10.wav"}
	if len(tracks) != len(want) {
		t.Fatalf("tracks=%v", tracks)
	}
	for i, w := range want {
		if filepath.Base(tracks[i]) != w {
			t.Fatalf("track[%d]=%q want %q (full=%v)", i, filepath.Base(tracks[i]), w, tracks)
		}
	}
}

func TestMOHTracksMissingDir(t *testing.T) {
	_, err := MOHTracks(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestMOHTracksParentFallback(t *testing.T) {
	dir := t.TempDir()
	mohDir := filepath.Join(dir, "moh")
	if err := os.Mkdir(mohDir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentWav := filepath.Join(dir, "moh.wav")
	if err := os.WriteFile(parentWav, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tracks, err := MOHTracks(mohDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0] != parentWav {
		t.Fatalf("tracks=%v want %q", tracks, parentWav)
	}
}
