package config

import (
	"path/filepath"
	"strings"
)

// Default prompt filenames (Asterisk core/extra sounds, English).
const (
	defaultSoundsDir        = "assets/sounds"
	defaultSoundBusy        = "is-curntly-busy.wav"
	defaultSoundWrongNumber = "you-dialed-wrong-number.wav"
	defaultSoundConfPIN     = "confbridge-pin.wav"
	defaultSoundConfPINBad  = "confbridge-pin-bad.wav"
	defaultSoundUnavailable = "vm-nobodyavail.wav"
	defaultSoundExtension   = "extension.wav"
)

func setSoundDefaults(s *SoundsConfig) {
	if s.Dir == "" {
		s.Dir = defaultSoundsDir
	}
	if s.Busy == "" {
		s.Busy = defaultSoundBusy
	}
	if s.WrongNumber == "" {
		s.WrongNumber = defaultSoundWrongNumber
	}
	if s.ConfPIN == "" {
		s.ConfPIN = defaultSoundConfPIN
	}
	if s.ConfPINBad == "" {
		s.ConfPINBad = defaultSoundConfPINBad
	}
	if s.Unavailable == "" {
		s.Unavailable = defaultSoundUnavailable
	}
	if s.Extension == "" {
		s.Extension = defaultSoundExtension
	}
}

// SoundPath resolves a prompt filename to an absolute-ish path under the sounds
// directory. cfgDir is the config file's directory (used for relative bases).
// Returns "" when the filename is empty (prompt disabled).
func (s SoundsConfig) SoundPath(cfgDir, filename string) string {
	if filename == "" {
		return ""
	}
	if filepath.IsAbs(filename) {
		return filename
	}
	dir := s.Dir
	if dir == "" {
		dir = defaultSoundsDir
	}
	if !filepath.IsAbs(dir) && cfgDir != "" && !strings.HasPrefix(dir, cfgDir) {
		dir = filepath.Join(cfgDir, dir)
	}
	return filepath.Join(dir, filename)
}
