package call

import (
	"context"
	"fmt"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/registrar"
)

// InviteContact places an outbound INVITE using the registered contact destination.
func InviteContact(ctx context.Context, dg *diago.Diago, recipient sip.Uri, destination, transport string, opts diago.InviteOptions, onMediaUpdate func(*diago.DialogMedia)) (*diago.DialogClientSession, error) {
	if transport == "" {
		transport = opts.Transport
	}
	if transport == "" && recipient.UriParams != nil {
		if t, ok := recipient.UriParams.Get("transport"); ok {
			transport = t
		}
	}
	d, err := dg.NewDialog(recipient, diago.NewDialogOptions{Transport: transport})
	if err != nil {
		return nil, err
	}
	if destination == "" {
		destination = recipient.HostPort()
	}
	d.InviteRequest.SetDestination(destination)

	// NOTE: We deliberately do NOT pass Originator here. diago's Originator path
	// collapses the outbound SDP offer to the caller's single negotiated codec to
	// avoid transcoding in its native proxy. This PBX bridges every call through an
	// always-on PCM transcoding hub, so the outbound leg must offer our full codec
	// set — otherwise a caller on PCMA could never reach a callee that only speaks
	// PCMU (no common codec -> 480). Mixed codecs are handled by the hub (REQ-BRIDGE-5).
	if err := d.Invite(ctx, diago.InviteClientOptions{
		OnResponse:    opts.OnResponse,
		Headers:       opts.Headers,
		Username:      opts.Username,
		Password:      opts.Password,
		OnMediaUpdate: onMediaUpdate,
	}); err != nil {
		_ = d.Close()
		return nil, err
	}
	if err := d.Ack(ctx); err != nil {
		_ = d.Close()
		return nil, err
	}
	EnsureClientDTMFCodec(d)
	return d, nil
}

// DiagoDialer wraps diago for the Dialer interface with correct contact routing.
type DiagoDialer struct {
	DG *diago.Diago
}

func (d *DiagoDialer) Invite(ctx context.Context, recipient sip.Uri, opts diago.InviteOptions) (*diago.DialogClientSession, error) {
	return InviteContact(ctx, d.DG, recipient, recipient.HostPort(), opts.Transport, opts, nil)
}

// ContactDialer carries explicit registration routing for outbound INVITEs.
type ContactDialer struct {
	DG *diago.Diago
}

func (d *ContactDialer) Invite(ctx context.Context, recipient sip.Uri, opts diago.InviteOptions) (*diago.DialogClientSession, error) {
	dest := recipient.HostPort()
	transport := opts.Transport
	return InviteContact(ctx, d.DG, recipient, dest, transport, opts, nil)
}

func (d *ContactDialer) InviteRegistered(ctx context.Context, recipient sip.Uri, destination, transport string, opts diago.InviteOptions, onMediaUpdate func(*diago.DialogMedia)) (*diago.DialogClientSession, error) {
	return InviteContact(ctx, d.DG, recipient, destination, transport, opts, onMediaUpdate)
}

// InviteExtension dials a registered extension using stored contact routing.
func InviteExtension(ctx context.Context, dg Dialer, reg *registrar.Registrar, ext string, inviteOpts diago.InviteOptions, onMediaUpdate func(*diago.DialogMedia)) (*diago.DialogClientSession, error) {
	uri, dest, transport, ok := reg.DialTarget(ext)
	if !ok {
		return nil, fmt.Errorf("extension %s unregistered", ext)
	}
	return InviteOutLeg(ctx, dg, uri, ConnectOpts{
		DialDestination: dest,
		DialTransport:   transport,
	}, inviteOpts, onMediaUpdate)
}

// InviteOutLeg sends an outbound INVITE with optional registered contact routing.
func InviteOutLeg(ctx context.Context, dg Dialer, outURI sip.Uri, opts ConnectOpts, inviteOpts diago.InviteOptions, onMediaUpdate func(*diago.DialogMedia)) (*diago.DialogClientSession, error) {
	dest := opts.DialDestination
	transport := opts.DialTransport
	if dest != "" {
		if cd, ok := dg.(*ContactDialer); ok {
			return cd.InviteRegistered(ctx, outURI, dest, transport, inviteOpts, onMediaUpdate)
		}
		if d, ok := dg.(*diago.Diago); ok {
			return InviteContact(ctx, d, outURI, dest, transport, inviteOpts, onMediaUpdate)
		}
	}
	return dg.Invite(ctx, outURI, inviteOpts)
}
