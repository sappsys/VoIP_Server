package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotTo(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	dst := filepath.Join(dir, "snap.db")
	st, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateHuntGroup("Sales", "500", "simultaneous", 20); err != nil {
		t.Fatal(err)
	}
	if err := st.SnapshotTo(dst); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	snap, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	groups, err := snap.ListHuntGroups()
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
}

func TestReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pbx.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateHuntGroup("A", "500", "simultaneous", 20); err != nil {
		t.Fatal(err)
	}
	if err := st.Reopen(path); err != nil {
		t.Fatal(err)
	}
	groups, err := st.ListHuntGroups()
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
	_ = st.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
