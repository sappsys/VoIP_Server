package presence

import (
	"log/slog"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/registrar"
	"github.com/sappsys/VoIP_Server/internal/router"
)

func TestSubscriptionStoreUpsertAndForWatched(t *testing.T) {
	store := NewSubscriptionStore()
	sub := &Subscription{
		CallID:      "abc",
		WatcherFrom: "tag1",
		Watcher:     "101",
		Watched:     "102",
		Event:       "presence",
		Expires:     time.Now().Add(time.Hour),
	}
	store.Upsert(sub)
	list := store.ForWatched("102")
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	store.Remove("abc", "tag1")
	if len(store.ForWatched("102")) != 0 {
		t.Fatal("expected removed")
	}
}

func TestHandleSubscribeRejectsBadEvent(t *testing.T) {
	reg := registrar.New("test", config.ServerConfig{}, map[string]*config.Extension{
		"101": {Extension: "101", Password: "x", Enabled: true},
		"102": {Extension: "102", Password: "x", Enabled: true},
	}, slog.Default())
	reg.RegisterForTest("101", sip.Uri{User: "101", Host: "127.0.0.1", Port: 15060})

	h := New(reg, map[string]*config.Extension{
		"101": {Extension: "101", Enabled: true},
		"102": {Extension: "102", Enabled: true},
	}, router.DefaultFeatureCodes(), "test.local", slog.Default())

	req := sip.NewRequest(sip.SUBSCRIBE, sip.Uri{User: "102", Host: "test.local"})
	req.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1", Port: 15060}, Params: sip.NewParams()})
	req.From().Params.Add("tag", "fromtag")
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "102", Host: "test.local"}})
	req.AppendHeader(sip.NewHeader("Call-ID", "call-1"))
	req.AppendHeader(sip.NewHeader("Event", "dialog"))
	req.AppendHeader(sip.NewHeader("Expires", "3600"))
	req.AppendHeader(&sip.ContactHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1", Port: 15060}})
	req.SetSource("127.0.0.1:15060")

	tx := &fakeServerTx{}
	h.handleSubscribe(req, tx)
	if tx.res == nil || tx.res.StatusCode != 489 {
		t.Fatalf("status=%d", statusCode(tx.res))
	}
}

type fakeServerTx struct {
	res *sip.Response
}

func (f *fakeServerTx) Respond(res *sip.Response) error {
	f.res = res
	return nil
}

func (f *fakeServerTx) Acks() <-chan *sip.Request       { return nil }
func (f *fakeServerTx) Done() <-chan struct{}          { return nil }
func (f *fakeServerTx) Err() error                     { return nil }
func (f *fakeServerTx) Terminate()                     {}
func (f *fakeServerTx) OnTerminate(sip.FnTxTerminate) bool { return false }
func (f *fakeServerTx) OnCancel(sip.FnTxCancel) bool       { return false }

func statusCode(res *sip.Response) int {
	if res == nil {
		return 0
	}
	return res.StatusCode
}
