package store

import "database/sql"

// SetLastDialed records the last number an extension dialed.
func (s *Store) SetLastDialed(extension, number string) error {
	_, err := s.db.Exec(`INSERT INTO extension_call_history(extension, last_dialed, last_caller)
		VALUES(?, ?, '')
		ON CONFLICT(extension) DO UPDATE SET last_dialed=excluded.last_dialed`,
		extension, number)
	return err
}

// SetLastCaller records the last caller to an extension.
func (s *Store) SetLastCaller(extension, caller string) error {
	_, err := s.db.Exec(`INSERT INTO extension_call_history(extension, last_dialed, last_caller)
		VALUES(?, '', ?)
		ON CONFLICT(extension) DO UPDATE SET last_caller=excluded.last_caller`,
		extension, caller)
	return err
}

// GetLastDialed returns the last number dialed by an extension.
func (s *Store) GetLastDialed(extension string) (string, error) {
	var n string
	err := s.db.QueryRow(`SELECT last_dialed FROM extension_call_history WHERE extension=?`, extension).Scan(&n)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return n, nil
}

// GetLastCaller returns the last caller to an extension.
func (s *Store) GetLastCaller(extension string) (string, error) {
	var n string
	err := s.db.QueryRow(`SELECT last_caller FROM extension_call_history WHERE extension=?`, extension).Scan(&n)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return n, nil
}
