package store

import "testing"

func TestOfflineMessagesEnqueueAndDeliver(t *testing.T) {
	st := openTestDB(t)
	if err := st.EnqueueOfflineMessage("102", "101", "text/plain", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	pending, err := st.ListPendingOfflineMessages("102")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || string(pending[0].Body) != "hi" {
		t.Fatalf("pending=%+v", pending)
	}
	if err := st.MarkOfflineMessageDelivered(pending[0].ID); err != nil {
		t.Fatal(err)
	}
	n, err := st.CountPendingOfflineMessages("102")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("pending count=%d", n)
	}
}
