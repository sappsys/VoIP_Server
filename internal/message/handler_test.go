package message

import (
	"log/slog"
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/registrar"
	"github.com/sappsys/VoIP_Server/internal/router"
)

func TestBuildForwardRequestCopiesBodyAndHeaders(t *testing.T) {
	from := &sip.FromHeader{Address: sip.Uri{User: "101", Host: "pbx.local"}}
	to := &sip.ToHeader{Address: sip.Uri{User: "102", Host: "pbx.local"}}
	in := sip.NewRequest(sip.MESSAGE, sip.Uri{User: "102", Host: "pbx.local"})
	in.AppendHeader(from)
	in.AppendHeader(to)
	in.AppendHeader(sip.NewHeader("Content-Type", "text/plain"))
	in.SetBody([]byte("hello"))

	out := buildForwardRequest(in, sip.Uri{User: "102", Host: "192.168.1.20", Port: 5060})
	if out.Method != sip.MESSAGE {
		t.Fatalf("method=%s", out.Method)
	}
	if out.Recipient.User != "102" || out.Recipient.Host != "192.168.1.20" {
		t.Fatalf("recipient=%v", out.Recipient)
	}
	if string(out.Body()) != "hello" {
		t.Fatalf("body=%q", out.Body())
	}
	if out.GetHeader("Content-Type") == nil {
		t.Fatal("missing content-type")
	}
}

func TestHandleMessageRejectsUnknownTarget(t *testing.T) {
	reg := registrar.New("test", config.ServerConfig{}, map[string]*config.Extension{
		"101": {Extension: "101", Password: "x", Enabled: true},
	}, slog.Default())
	reg.RegisterForTest("101", sip.Uri{User: "101", Host: "127.0.0.1", Port: 15060})

	h := New(reg, map[string]*config.Extension{
		"101": {Extension: "101", Enabled: true},
	}, router.DefaultFeatureCodes(), "test.local", slog.Default())

	req := sip.NewRequest(sip.MESSAGE, sip.Uri{User: "999", Host: "pbx.local"})
	req.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1", Port: 15060}})
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "999", Host: "pbx.local"}})
	req.SetBody([]byte("hi"))
	req.SetSource("127.0.0.1:15060")

	tx := &fakeServerTx{req: req}
	h.handleMessage(req, tx)
	if tx.res == nil || tx.res.StatusCode != sip.StatusNotFound {
		t.Fatalf("status=%d", statusCode(tx.res))
	}
}

func TestHandleMessageRejectsUnauthorizedSender(t *testing.T) {
	reg := registrar.New("test", config.ServerConfig{}, map[string]*config.Extension{
		"101": {Extension: "101", Password: "x", Enabled: true},
		"102": {Extension: "102", Password: "x", Enabled: true},
	}, slog.Default())
	reg.RegisterForTest("101", sip.Uri{User: "101", Host: "127.0.0.1", Port: 15060})
	reg.RegisterForTest("102", sip.Uri{User: "102", Host: "127.0.0.1", Port: 15061})

	h := New(reg, map[string]*config.Extension{
		"101": {Extension: "101", Enabled: true},
		"102": {Extension: "102", Enabled: true},
	}, router.DefaultFeatureCodes(), "test.local", slog.Default())

	req := sip.NewRequest(sip.MESSAGE, sip.Uri{User: "102", Host: "pbx.local"})
	req.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1", Port: 15060}})
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "102", Host: "pbx.local"}})
	req.SetBody([]byte("hi"))
	req.SetSource("10.0.0.9:5060")

	tx := &fakeServerTx{req: req}
	h.handleMessage(req, tx)
	if tx.res == nil || tx.res.StatusCode != sip.StatusForbidden {
		t.Fatalf("status=%d", statusCode(tx.res))
	}
}

type fakeServerTx struct {
	req *sip.Request
	res *sip.Response
}

func (f *fakeServerTx) Respond(res *sip.Response) error {
	f.res = res
	return nil
}

func (f *fakeServerTx) Acks() <-chan *sip.Request { return nil }
func (f *fakeServerTx) Done() <-chan struct{}    { return nil }
func (f *fakeServerTx) Err() error                 { return nil }
func (f *fakeServerTx) Terminate()                 {}
func (f *fakeServerTx) OnTerminate(sip.FnTxTerminate) bool { return false }
func (f *fakeServerTx) OnCancel(sip.FnTxCancel) bool     { return false }

func statusCode(res *sip.Response) int {
	if res == nil {
		return 0
	}
	return res.StatusCode
}
