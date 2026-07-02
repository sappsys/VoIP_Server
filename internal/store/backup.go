package store

import (
	"database/sql"
	"fmt"
)

// SnapshotTo writes a consistent copy of the database to dest using VACUUM INTO.
func (s *Store) SnapshotTo(dest string) error {
	_, err := s.db.Exec(fmt.Sprintf("VACUUM INTO %q", dest))
	return err
}

// Reopen closes the current database and opens path.
func (s *Store) Reopen(path string) error {
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	s.db = db
	return s.migrate()
}
