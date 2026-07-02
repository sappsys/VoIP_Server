package store

import (
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCallHistory(t *testing.T) {
	st := openTestDB(t)

	if err := st.SetLastDialed("101", "102"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetLastCaller("102", "101"); err != nil {
		t.Fatal(err)
	}

	dialed, err := st.GetLastDialed("101")
	if err != nil || dialed != "102" {
		t.Fatalf("last dialed: %q err=%v", dialed, err)
	}
	caller, err := st.GetLastCaller("102")
	if err != nil || caller != "101" {
		t.Fatalf("last caller: %q err=%v", caller, err)
	}

	if err := st.SetLastDialed("101", "103"); err != nil {
		t.Fatal(err)
	}
	dialed, _ = st.GetLastDialed("101")
	if dialed != "103" {
		t.Fatalf("update dialed: %q", dialed)
	}

	missing, err := st.GetLastDialed("999")
	if err != nil || missing != "" {
		t.Fatalf("missing ext: %q err=%v", missing, err)
	}
}

func TestHuntGroupCRUD(t *testing.T) {
	st := openTestDB(t)
	if err := st.CreateHuntGroup("Sales", "501", "simultaneous", 25); err != nil {
		t.Fatal(err)
	}
	g, err := st.GetHuntGroupByNumber("501")
	if err != nil || g == nil || g.Name != "Sales" {
		t.Fatalf("get hunt: %+v err=%v", g, err)
	}
	if err := st.SetHuntMembers(g.ID, []string{"101", "102"}); err != nil {
		t.Fatal(err)
	}
	members, err := st.HuntMembers(g.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members: %v err=%v", members, err)
	}
}

func TestTrunkRouteSave(t *testing.T) {
	st := openTestDB(t)
	if err := st.SaveTrunkRoute(TrunkRoute{TrunkID: 1, RouteType: "extension", RouteTarget: "101"}); err != nil {
		t.Fatal(err)
	}
	r, err := st.GetTrunkRoute(1)
	if err != nil || r.RouteTarget != "101" {
		t.Fatalf("route: %+v err=%v", r, err)
	}
}

func TestConferenceCRUD(t *testing.T) {
	st := openTestDB(t)
	if err := st.CreateConference("Standup", "600", "1234", 8); err != nil {
		t.Fatal(err)
	}
	c, err := st.GetConferenceByNumber("600")
	if err != nil || c == nil || c.Name != "Standup" {
		t.Fatalf("conference: %+v err=%v", c, err)
	}
	if c.MaxParticipants != 8 {
		t.Fatalf("max=%d", c.MaxParticipants)
	}
}

func TestPagingGroupCRUD(t *testing.T) {
	st := openTestDB(t)
	if err := st.CreatePagingGroup("All Hands", "88", "unicast", "", 0); err != nil {
		t.Fatal(err)
	}
	g, err := st.GetPagingByCode("88")
	if err != nil || g == nil || g.Name != "All Hands" {
		t.Fatalf("paging: %+v err=%v", g, err)
	}
	if err := st.SetPagingMembers(g.ID, []string{"101", "102"}); err != nil {
		t.Fatal(err)
	}
	members, err := st.PagingMembers(g.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members: %v err=%v", members, err)
	}
}

func TestWebUserUpsert(t *testing.T) {
	st := openTestDB(t)
	hash, err := HashPassword("admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertWebUser("admin", hash, "admin"); err != nil {
		t.Fatal(err)
	}
	u, err := st.GetWebUserByUsername("admin")
	if err != nil || u == nil || u.Role != "admin" {
		t.Fatalf("user: %+v err=%v", u, err)
	}
}
