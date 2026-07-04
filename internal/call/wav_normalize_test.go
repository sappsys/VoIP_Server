package call

import (
	"os"
	"path/filepath"
	"testing"

	diagomedia "github.com/emiago/diago/media"
)

func TestResampleLinear(t *testing.T) {
	in := []int16{0, 1000, 2000, 3000}
	out := resampleLinear(in, 8000, 4000)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	if out[0] != 0 || out[1] != 2000 {
		t.Fatalf("out=%v", out)
	}
}

func TestDownmixToMono(t *testing.T) {
	stereo := []int16{1000, 3000, 2000, 4000}
	mono, err := downmixToMono(stereo, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(mono) != 2 || mono[0] != 2000 || mono[1] != 3000 {
		t.Fatalf("mono=%v", mono)
	}
}

func TestLoadWavPCMForCodecMOH(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "moh", "moh.wav")
	if _, err := os.Stat(path); err != nil {
		t.Skip("moh.wav not present")
	}
	codec := diagomedia.CodecAudioUlaw
	pcm, err := loadWavPCMForCodec(path, codec)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) < 8000*2 {
		t.Fatalf("pcm too short: %d bytes", len(pcm))
	}
	if len(pcm)%2 != 0 {
		t.Fatal("pcm not 16-bit aligned")
	}
}

func TestOpenWavPlaybackReaderMOHUsesPCM(t *testing.T) {
	path := filepath.Join("..", "..", "assets", "moh", "moh.wav")
	if _, err := os.Stat(path); err != nil {
		t.Skip("moh.wav not present")
	}
	_, mime, err := openWavPlaybackReader(path, diagomedia.CodecAudioUlaw)
	if err != nil {
		t.Fatal(err)
	}
	if mime != "audio/pcm" && mime != "audio/wav" {
		t.Fatalf("mime=%q want audio/pcm or audio/wav for moh playback", mime)
	}
}

func TestWavMatchesCodec(t *testing.T) {
	info := wavPCMInfo{sampleRate: 8000, numChannels: 1, bitsPerSample: 16}
	if !wavMatchesCodec(info, diagomedia.CodecAudioUlaw) {
		t.Fatal("expected match")
	}
	info.sampleRate = 44100
	if wavMatchesCodec(info, diagomedia.CodecAudioUlaw) {
		t.Fatal("expected mismatch")
	}
}
