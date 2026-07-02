package store

import "testing"

func TestPhonebookCRUD(t *testing.T) {
	st := openTestDB(t)

	id, err := st.CreatePhonebookEntry("Alice", "101", "Office")
	if err != nil || id == 0 {
		t.Fatalf("create: id=%d err=%v", id, err)
	}
	if _, err := st.CreatePhonebookEntry("Bob", "sip:102@host", ""); err != nil {
		t.Fatal(err)
	}

	list, err := st.ListPhonebookEntries()
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v err=%v", list, err)
	}
	// Ordered by name COLLATE NOCASE: Alice before Bob.
	if list[0].Name != "Alice" || list[1].Name != "Bob" {
		t.Fatalf("order wrong: %+v", list)
	}

	if err := st.UpdatePhonebookEntry(id, "Alice Smith", "111", "Mobile"); err != nil {
		t.Fatal(err)
	}
	e, err := st.GetPhonebookEntry(id)
	if err != nil || e == nil {
		t.Fatalf("get: %+v err=%v", e, err)
	}
	if e.Name != "Alice Smith" || e.Number != "111" || e.Label != "Mobile" {
		t.Fatalf("update not applied: %+v", e)
	}

	if err := st.DeletePhonebookEntry(id); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetPhonebookEntry(id); got != nil {
		t.Fatalf("expected deleted, got %+v", got)
	}
	remaining, _ := st.ListPhonebookEntries()
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(remaining))
	}
}

func TestGetPhonebookEntryMissing(t *testing.T) {
	st := openTestDB(t)
	e, err := st.GetPhonebookEntry(999)
	if err != nil {
		t.Fatal(err)
	}
	if e != nil {
		t.Fatalf("expected nil for missing id, got %+v", e)
	}
}
