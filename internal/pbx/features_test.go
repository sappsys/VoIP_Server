package pbx

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/sappsys/VoIP_Server/internal/call"
)

func TestHandleTransferSetsReady(t *testing.T) {
	srv, _, _ := newTestServerLight(t)
	ac := &call.ActiveCall{
		CallerExt: "101", CalleeExt: "102",
		In: testInviteDialog("active-1", "101", "102"),
	}
	srv.registry.Register(ac)

	if !srv.registry.SetTransferReady("101") {
		t.Fatal("expected transfer ready")
	}
	if !ac.TransferReady {
		t.Fatal("active call not marked transfer ready")
	}
}

func TestHandleParkCreatesSlot(t *testing.T) {
	srv, _, _ := newTestServerLight(t)
	ac := &call.ActiveCall{
		CallerExt: "101", CalleeExt: "102",
		In: testInviteDialog("park-active", "101", "102"),
	}
	srv.registry.Register(ac)

	heldIn, heldOut := ac.StealHeldLeg("101")
	srv.park.Park("101", heldIn, heldOut, nil)

	if !srv.park.Has("101") {
		t.Fatal("expected park slot 101")
	}
}

func TestHandleParkRetrieveEmptySlot(t *testing.T) {
	srv, _, _ := newTestServerLight(t)
	if pc := srv.park.Retrieve("101"); pc != nil {
		t.Fatal("expected nil for empty slot")
	}
}

func TestBridgeToExtensionDNDNoDial(t *testing.T) {
	srv, _, md := newTestServerLight(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	in := testInviteDialog("dnd-call", "101", "103")
	opts := srv.connectOpts("101", "103")
	_ = srv.bridgeToExtension(ctx, in, "103", opts)

	if md.total() != 0 {
		t.Fatalf("DND must not dial callee, invites=%d", md.total())
	}
}

func TestHandleHuntAllDNDNoDial(t *testing.T) {
	srv, st, md := newTestServerLight(t)
	_ = st.CreateHuntGroup("G", "501", "simultaneous", 5)
	g, _ := st.GetHuntGroupByNumber("501")
	_ = st.SetHuntMembers(g.ID, []string{"103"})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	in := testInviteDialog("hunt-dnd", "101", "501")
	opts := srv.connectOpts("101", "501")
	_ = srv.handleHunt(ctx, in, "501", opts)

	if md.total() != 0 {
		t.Fatalf("all-DND hunt must not dial, invites=%d", md.total())
	}
}

func TestDialResolvedExtensionInvites(t *testing.T) {
	srv, _, md := newTestServerLight(t)
	ctx := context.Background()

	uri, ok := srv.reg.ContactURI("102")
	if !ok {
		t.Fatal("102 not registered")
	}
	_, err := srv.inviteDialer().Invite(ctx, uri, diago.InviteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if md.count("102") != 1 {
		t.Fatalf("expected 1 invite to 102, got %d", md.count("102"))
	}
}

func TestRegistryConsultLink(t *testing.T) {
	srv, _, _ := newTestServerLight(t)
	ac := &call.ActiveCall{
		CallerExt: "101",
		In:        testInviteDialog("main", "101", "102"),
	}
	srv.registry.Register(ac)
	consult := testInviteDialog("consult", "101", "103")
	srv.registry.SetConsult("101", consult)
	if ac.ConsultIn != consult {
		t.Fatal("consult not linked")
	}
}
