package presence

// Basic PIDF presence values (RFC 3863).
const (
	BasicOpen   = "open"
	BasicClosed = "closed"
	BasicBusy   = "busy"
)

// State is the presence of one extension.
type State struct {
	Basic       string
	DisplayName string
}

func (s State) Online() bool { return s.Basic != BasicClosed }
