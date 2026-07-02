package pbx

import "testing"

func TestStatusReport(t *testing.T) {
	s, st, _ := newTestServerLight(t)
	s.logCall("101", "102", "", "internal", "", "")

	stt := s.Status()
	if stt.RegisteredCount != 2 {
		t.Fatalf("registered: %d", stt.RegisteredCount)
	}
	if len(stt.Extensions) != 3 {
		t.Fatalf("extensions: %d", len(stt.Extensions))
	}
	var alice, carol bool
	for _, e := range stt.Extensions {
		switch e.Extension {
		case "101":
			alice = e.Registered
		case "103":
			if !e.DND {
				t.Fatal("103 should be DND")
			}
			carol = true
		}
	}
	if !alice {
		t.Fatal("101 not registered in status")
	}
	if !carol {
		t.Fatal("103 missing from status")
	}

	log, err := st.ListCallLog(5)
	if err != nil || len(log) != 1 || log[0].Direction != "internal" {
		t.Fatalf("call log: %+v err=%v", log, err)
	}
}
