package pbx

import (
	"github.com/sappsys/VoIP_Server/internal/presence"
)

func (s *Server) presenceState(ext string) presence.State {
	if e, ok := s.exts[ext]; ok {
		if !e.Enabled {
			return presence.State{Basic: presence.BasicClosed, DisplayName: e.DisplayName}
		}
		if !s.reg.IsRegistered(ext) {
			return presence.State{Basic: presence.BasicClosed, DisplayName: e.DisplayName}
		}
		if e.DND || (s.calls != nil && s.calls.ExtensionActive(ext) > 0) {
			return presence.State{Basic: presence.BasicBusy, DisplayName: e.DisplayName}
		}
		return presence.State{Basic: presence.BasicOpen, DisplayName: e.DisplayName}
	}
	if s.reg.IsRegistered(ext) {
		return presence.State{Basic: presence.BasicOpen}
	}
	return presence.State{Basic: presence.BasicClosed}
}
