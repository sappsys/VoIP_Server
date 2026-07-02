package presence

import "testing"

func TestPIDFXML(t *testing.T) {
	body := string(pidfXML("sip:102@test.local", "102", BasicOpen, "Bob"))
	if body == "" {
		t.Fatal("empty pidf")
	}
	if !contains(body, "<basic>open</basic>") {
		t.Fatalf("body=%s", body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
