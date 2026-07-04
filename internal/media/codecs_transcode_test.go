package media

import "testing"

func TestTranscodeSupported(t *testing.T) {
	for _, id := range []string{"PCMU", "G722", "G729", "G726-32"} {
		if !TranscodeSupported(id) {
			t.Fatalf("%s should transcode", id)
		}
	}
	if TranscodeSupported("G723-63") {
		t.Fatal("g723 should not transcode yet")
	}
}
