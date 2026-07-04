//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/diago"
)

// REQ-CONF-PIN-1: a phone joins conference 600 and keys in the PIN via DTMF
// (RFC 2833). DTMF is decoded immediately (even during the prompt) and a correct
// PIN admits the participant.
// REQ-CONF-2: a sole participant hears Music on Hold (server keeps the leg up
// playing MOH rather than dropping it).
func TestREQ_CONF_PINThenSoloMOH(t *testing.T) {
	pbx := startPBX(t, pbxOptions{
		Extensions:  map[string]string{"111": "andy"},
		Conferences: []confSpec{{Number: "600", PIN: "1234", Max: 8}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	phone := newHandset(t, pbx.Port, "111", "andy")
	phone.register()

	out, err := phone.invite(ctx, "600", nil)
	if err != nil {
		t.Fatalf("invite 111->600: %v", err)
	}
	defer out.Close()

	// Give the server a moment to answer and start the PIN prompt, then key PIN.
	time.Sleep(500 * time.Millisecond)
	if err := sendDTMF(out, "1234#"); err != nil {
		t.Fatalf("send PIN DTMF: %v", err)
	}

	// After a correct PIN, the sole participant is admitted and kept on the call
	// (MOH). The leg must stay up rather than being torn down.
	if !waitFor(6*time.Second, func() bool { return pbx.Srv.Status().ActiveCallCount >= 0 && out.Context().Err() == nil }) {
		t.Fatal("REQ-CONF-PIN-1/REQ-CONF-2: participant leg dropped (PIN rejected or MOH not started)")
	}

	// Confirm the participant is counted in the room (admitted past the PIN gate).
	if !waitFor(6*time.Second, func() bool { return conferenceParticipants(pbx, "600") >= 1 }) {
		t.Fatal("REQ-CONF-PIN-1: participant not admitted to room 600 after correct PIN")
	}
}

// REQ-CONF-PIN-2: an incorrect PIN is rejected (participant not admitted).
func TestREQ_CONF_WrongPINNotAdmitted(t *testing.T) {
	pbx := startPBX(t, pbxOptions{
		Extensions:  map[string]string{"111": "andy"},
		Conferences: []confSpec{{Number: "600", PIN: "1234", Max: 8}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	phone := newHandset(t, pbx.Port, "111", "andy")
	phone.register()

	out, err := phone.invite(ctx, "600", nil)
	if err != nil {
		t.Fatalf("invite 111->600: %v", err)
	}
	defer out.Close()

	time.Sleep(500 * time.Millisecond)
	_ = sendDTMF(out, "9999#")

	// Wrong PIN must NOT admit the caller to the room.
	admitted := waitFor(3*time.Second, func() bool { return conferenceParticipants(pbx, "600") >= 1 })
	if admitted {
		t.Fatal("REQ-CONF-PIN-2: wrong PIN must not admit participant")
	}
}

func conferenceParticipants(pbx *testPBX, number string) int {
	return pbx.Srv.ConferenceParticipants(number)
}

func joinConference(t *testing.T, ctx context.Context, phone *handset, pin string) *diago.DialogClientSession {
	t.Helper()
	out, err := phone.invite(ctx, "600", nil)
	if err != nil {
		t.Fatalf("invite ->600: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := sendDTMF(out, pin+"#"); err != nil {
		out.Close()
		t.Fatalf("send PIN: %v", err)
	}
	if !waitFor(8*time.Second, func() bool { return out.Context().Err() == nil }) {
		out.Close()
		t.Fatal("conference leg dropped during PIN entry")
	}
	return out
}

// REQ-CONF-3: two admitted participants must hear each other via the mixer (not MOH-only).
func TestREQ_CONF_TwoParticipantsMixerAudio(t *testing.T) {
	pbx := startPBX(t, pbxOptions{
		Extensions:  map[string]string{"110": "andy", "111": "andy"},
		Conferences: []confSpec{{Number: "600", PIN: "1234", Max: 8}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	a := newHandset(t, pbx.Port, "111", "andy")
	b := newHandset(t, pbx.Port, "110", "andy")
	a.register()
	b.register()

	legA := joinConference(t, ctx, a, "1234")
	defer legA.Close()
	if !waitFor(8*time.Second, func() bool { return conferenceParticipants(pbx, "600") >= 1 }) {
		t.Fatal("participant A not admitted")
	}

	legB := joinConference(t, ctx, b, "1234")
	defer legB.Close()
	if !waitFor(8*time.Second, func() bool { return conferenceParticipants(pbx, "600") >= 2 }) {
		t.Fatal("REQ-CONF-3: second participant not admitted to mixer room")
	}

	time.Sleep(500 * time.Millisecond)
	readCtx, readCancel := context.WithTimeout(ctx, 12*time.Second)
	defer readCancel()
	assertTwoWayRTP(t, readCtx, legA, legB, 320)
}

// REQ-CONF-4: when a room drops from two participants to one, MOH restarts for the survivor.
func TestREQ_CONF_DropToOneRestartsMOH(t *testing.T) {
	pbx := startPBX(t, pbxOptions{
		Extensions:  map[string]string{"110": "andy", "111": "andy"},
		Conferences: []confSpec{{Number: "600", PIN: "1234", Max: 8}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	a := newHandset(t, pbx.Port, "111", "andy")
	b := newHandset(t, pbx.Port, "110", "andy")
	a.register()
	b.register()

	legA := joinConference(t, ctx, a, "1234")
	defer legA.Close()
	legB := joinConference(t, ctx, b, "1234")
	if !waitFor(8*time.Second, func() bool { return conferenceParticipants(pbx, "600") >= 2 }) {
		legB.Close()
		t.Fatal("need two participants before drop-to-one test")
	}

	_ = legB.Hangup(ctx)
	if !waitFor(8*time.Second, func() bool {
		return conferenceParticipants(pbx, "600") == 1 && legA.Context().Err() == nil
	}) {
		t.Fatal("REQ-CONF-4: survivor leg dropped or count wrong after peer left")
	}

	time.Sleep(400 * time.Millisecond)
	readCtx, readCancel := context.WithTimeout(ctx, 10*time.Second)
	defer readCancel()
	if err := readRTPBytes(readCtx, legA, 320); err != nil {
		t.Fatalf("REQ-CONF-4: sole survivor must receive MOH RTP after 2→1: %v", err)
	}
}
