package store

import "time"

// CallLogEntry is one row in the call audit log.
type CallLogEntry struct {
	ID          int64
	StartedAt   time.Time
	Caller      string
	Callee      string
	CallerName  string
	Direction   string // internal, inbound-trunk, outbound-trunk
	TrunkName   string
	TrunkPrefix string
}

// ExtensionHistory holds per-extension redial/return state.
type ExtensionHistory struct {
	Extension  string
	LastDialed string
	LastCaller string
}

// LogCall appends a call record to the audit log.
func (s *Store) LogCall(e CallLogEntry) error {
	if e.StartedAt.IsZero() {
		e.StartedAt = time.Now().UTC()
	}
	if e.Direction == "" {
		e.Direction = "internal"
	}
	_, err := s.db.Exec(`INSERT INTO call_log(started_at, caller, callee, caller_name, direction, trunk_name, trunk_prefix)
		VALUES(?,?,?,?,?,?,?)`,
		e.StartedAt.UTC().Format(time.RFC3339), e.Caller, e.Callee, e.CallerName, e.Direction, e.TrunkName, e.TrunkPrefix)
	return err
}

// ListCallLog returns recent call log entries, newest first.
func (s *Store) ListCallLog(limit int) ([]CallLogEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT id, started_at, caller, callee, caller_name, direction, trunk_name, trunk_prefix
		FROM call_log ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CallLogEntry
	for rows.Next() {
		var e CallLogEntry
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Caller, &e.Callee, &e.CallerName, &e.Direction, &e.TrunkName, &e.TrunkPrefix); err != nil {
			return nil, err
		}
		e.StartedAt, _ = time.Parse(time.RFC3339, ts)
		list = append(list, e)
	}
	return list, rows.Err()
}

// ListExtensionHistory returns last-dialed/last-caller for all extensions with history rows.
func (s *Store) ListExtensionHistory() ([]ExtensionHistory, error) {
	rows, err := s.db.Query(`SELECT extension, last_dialed, last_caller FROM extension_call_history ORDER BY extension`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ExtensionHistory
	for rows.Next() {
		var h ExtensionHistory
		if err := rows.Scan(&h.Extension, &h.LastDialed, &h.LastCaller); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, rows.Err()
}
