package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEditLinksOnAdminPages(t *testing.T) {
	s, st, _ := testWebServer(t)
	cookie := loginAs(t, s, "admin")
	h := s.Handler()

	// Extension (file-backed).
	extForm := strings.NewReader("extension=101&display_name=Test&password=secret&max_simultaneous_calls=4&enabled=1&call_waiting=1")
	extReq := httptest.NewRequest(http.MethodPost, "/extensions/save", extForm)
	extReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	extReq.AddCookie(cookie)
	extRR := httptest.NewRecorder()
	s.handleExtensionSave(extRR, extReq)
	if extRR.Code != http.StatusSeeOther {
		t.Fatalf("extension save status=%d", extRR.Code)
	}

	id, err := st.CreatePhonebookEntry("Alice", "101", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = st.CreateHuntGroup("Sales", "500", "simultaneous", 20)
	_ = st.CreateConference("Main", "600", "1234", 8)
	_ = st.CreatePagingGroup("All", "80", "unicast", "", 0)

	users, _ := st.ListWebUsers()
	if len(users) == 0 {
		t.Fatal("expected web user from loginAs")
	}
	userID := users[0].ID

	pages := []string{
		"/extensions?edit=101",
		"/hunt?edit=1",
		"/conferences?edit=1",
		"/paging?edit=1",
		"/phonebook?edit=" + itoa(id),
		fmt.Sprintf("/users?edit=%d", userID),
	}
	for _, path := range pages {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, `id="edit-form"`) {
			t.Fatalf("%s missing edit form anchor", path)
		}
		if !strings.Contains(body, "Edit</a>") {
			t.Fatalf("%s missing edit link in list", path)
		}
	}
}

func TestPhonebookUpdateViaEditForm(t *testing.T) {
	s, st, _ := testWebServer(t)
	cookie := loginAs(t, s, "admin")

	id, err := st.CreatePhonebookEntry("Alice", "101", "")
	if err != nil {
		t.Fatal(err)
	}

	form := strings.NewReader("id=" + itoa(id) + "&name=Alice+Smith&number=111&label=Mobile")
	req := httptest.NewRequest(http.MethodPost, "/phonebook/save", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	s.handlePhonebookSave(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("save status=%d", rr.Code)
	}

	got, err := st.GetPhonebookEntry(id)
	if err != nil || got == nil {
		t.Fatalf("entry missing: %v", err)
	}
	if got.Name != "Alice Smith" || got.Number != "111" || got.Label != "Mobile" {
		t.Fatalf("unexpected entry: %+v", got)
	}
}
