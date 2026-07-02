package store

import (
	"time"
)

type OfflineMessage struct {
	ID          int64
	Recipient   string
	Sender      string
	ContentType string
	Body        []byte
	CreatedAt   time.Time
}

func (s *Store) EnqueueOfflineMessage(recipient, sender, contentType string, body []byte) error {
	if contentType == "" {
		contentType = "text/plain"
	}
	_, err := s.db.Exec(`INSERT INTO offline_messages(recipient, sender, content_type, body, created_at)
		VALUES(?,?,?,?,?)`,
		recipient, sender, contentType, body, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) ListPendingOfflineMessages(recipient string) ([]OfflineMessage, error) {
	rows, err := s.db.Query(`SELECT id, recipient, sender, content_type, body, created_at
		FROM offline_messages WHERE recipient=? AND delivered_at IS NULL ORDER BY id`, recipient)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []OfflineMessage
	for rows.Next() {
		var m OfflineMessage
		var created string
		if err := rows.Scan(&m.ID, &m.Recipient, &m.Sender, &m.ContentType, &m.Body, &created); err != nil {
			return nil, err
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, created)
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *Store) MarkOfflineMessageDelivered(id int64) error {
	_, err := s.db.Exec(`UPDATE offline_messages SET delivered_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func (s *Store) CountPendingOfflineMessages(recipient string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM offline_messages WHERE recipient=? AND delivered_at IS NULL`, recipient).
		Scan(&n)
	return n, err
}
