package store

import "database/sql"

// PhonebookEntry is a single contact in the shared remote phonebook.
// Label is optional and maps to the Yealink <Telephone label="..."> attribute.
type PhonebookEntry struct {
	ID     int64
	Name   string
	Number string
	Label  string
}

// ListPhonebookEntries returns all entries ordered by name for display and XML export.
func (s *Store) ListPhonebookEntries() ([]PhonebookEntry, error) {
	rows, err := s.db.Query(`SELECT id, name, number, label FROM phonebook_entries ORDER BY name COLLATE NOCASE, number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PhonebookEntry
	for rows.Next() {
		var e PhonebookEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.Number, &e.Label); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// GetPhonebookEntry returns a single entry by id, or nil if not found.
func (s *Store) GetPhonebookEntry(id int64) (*PhonebookEntry, error) {
	var e PhonebookEntry
	err := s.db.QueryRow(`SELECT id, name, number, label FROM phonebook_entries WHERE id=?`, id).
		Scan(&e.ID, &e.Name, &e.Number, &e.Label)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CreatePhonebookEntry inserts a new contact and returns its id.
func (s *Store) CreatePhonebookEntry(name, number, label string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO phonebook_entries(name, number, label) VALUES(?,?,?)`, name, number, label)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdatePhonebookEntry edits an existing contact.
func (s *Store) UpdatePhonebookEntry(id int64, name, number, label string) error {
	_, err := s.db.Exec(`UPDATE phonebook_entries SET name=?, number=?, label=? WHERE id=?`, name, number, label, id)
	return err
}

// DeletePhonebookEntry removes a contact by id.
func (s *Store) DeletePhonebookEntry(id int64) error {
	_, err := s.db.Exec(`DELETE FROM phonebook_entries WHERE id=?`, id)
	return err
}
