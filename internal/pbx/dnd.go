package pbx

import (
	"context"
	"fmt"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
)

func (s *Server) setDND(ext string, on bool) error {
	e, ok := s.exts[ext]
	if !ok {
		return fmt.Errorf("unknown extension %s", ext)
	}
	e.DND = on
	if err := config.SaveExtension(s.extDir, e); err != nil {
		return err
	}
	s.exts[ext] = e
	return nil
}

func (s *Server) handleDND(ctx context.Context, in *diago.DialogServerSession, from string, on bool) {
	if !s.reg.IsRegistered(from) {
		_ = in.Respond(sip.StatusForbidden, "Forbidden", nil)
		return
	}
	if _, ok := s.exts[from]; !ok {
		_ = in.Respond(sip.StatusNotFound, "Not Found", nil)
		return
	}
	if err := s.setDND(from, on); err != nil {
		_ = in.Respond(sip.StatusInternalServerError, "Error", nil)
		if s.log != nil {
			s.log.Warn("set dnd", "ext", from, "on", on, "error", err)
		}
		return
	}
	in.Trying()
	if err := in.Answer(); err != nil {
		return
	}
	in.Hangup(ctx)
	if s.log != nil {
		state := "off"
		if on {
			state = "on"
		}
		s.log.Info("dnd updated", "extension", from, "state", state)
	}
}

func (s *Server) filterDND(members []string) []string {
	out := make([]string, 0, len(members))
	for _, m := range members {
		if e, ok := s.exts[m]; ok && e.DND {
			continue
		}
		out = append(out, m)
	}
	return out
}
