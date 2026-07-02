package store

import "testing"

func TestCallLog(t *testing.T) {
	st := openTestDB(t)

	if err := st.LogCall(CallLogEntry{
		Caller: "101", Callee: "102", Direction: "internal",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.LogCall(CallLogEntry{
		Caller: "5551234", Callee: "101", Direction: "inbound-trunk",
		TrunkName: "PSTN", TrunkPrefix: "9",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.LogCall(CallLogEntry{
		Caller: "101", Callee: "+15551234", Direction: "outbound-trunk",
		TrunkName: "PSTN", TrunkPrefix: "9",
	}); err != nil {
		t.Fatal(err)
	}

	all, err := st.ListCallLog(10)
	if err != nil || len(all) != 3 {
		t.Fatalf("list all: %d err=%v", len(all), err)
	}
	if all[0].Direction != "outbound-trunk" {
		t.Fatalf("newest first: %+v", all[0])
	}

	var inbound int
	for _, e := range all {
		if e.Direction == "inbound-trunk" {
			inbound++
		}
	}
	if inbound != 1 {
		t.Fatalf("inbound count: %d", inbound)
	}
}
