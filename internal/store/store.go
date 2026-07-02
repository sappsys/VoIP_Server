package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type WebUser struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
}

type HuntGroup struct {
	ID                 int64
	Name               string
	Number             string
	Strategy           string
	RingTimeoutSeconds int
	Enabled            bool
}

type Conference struct {
	ID              int64
	Name            string
	Number          string
	PINHash         string
	MaxParticipants int
	Enabled         bool
}

type PagingGroup struct {
	ID               int64
	Name             string
	Code             string
	Mode             string
	MulticastAddress string
	Channel          int
	Enabled          bool
}

type TrunkRoute struct {
	TrunkID     int
	RouteType   string
	RouteTarget string
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.seedDefaultAdmin(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS web_users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'admin',
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS hunt_groups (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  number TEXT NOT NULL UNIQUE,
  strategy TEXT NOT NULL DEFAULT 'simultaneous',
  ring_timeout_seconds INTEGER NOT NULL DEFAULT 20,
  enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS hunt_group_members (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER NOT NULL,
  extension TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY(group_id) REFERENCES hunt_groups(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS conferences (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  number TEXT NOT NULL UNIQUE,
  pin_hash TEXT NOT NULL DEFAULT '',
  max_participants INTEGER NOT NULL DEFAULT 16,
  enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS paging_groups (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  code TEXT NOT NULL UNIQUE,
  mode TEXT NOT NULL DEFAULT 'unicast',
  multicast_address TEXT NOT NULL DEFAULT '',
  channel INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS paging_group_members (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER NOT NULL,
  extension TEXT NOT NULL,
  FOREIGN KEY(group_id) REFERENCES paging_groups(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS trunk_routes (
  trunk_id INTEGER PRIMARY KEY,
  route_type TEXT NOT NULL DEFAULT 'all',
  route_target TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS extension_call_history (
  extension TEXT PRIMARY KEY,
  last_dialed TEXT NOT NULL DEFAULT '',
  last_caller TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS phonebook_entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  number TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS call_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  started_at TEXT NOT NULL,
  caller TEXT NOT NULL,
  callee TEXT NOT NULL,
  caller_name TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL DEFAULT 'internal',
  trunk_name TEXT NOT NULL DEFAULT '',
  trunk_prefix TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_call_log_started ON call_log(started_at DESC);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	return s.migrateLegacy()
}

func (s *Store) migrateLegacy() error {
	alters := []string{
		`ALTER TABLE hunt_groups ADD COLUMN ring_timeout_seconds INTEGER NOT NULL DEFAULT 20`,
		`ALTER TABLE hunt_groups ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE conferences ADD COLUMN pin_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE conferences ADD COLUMN max_participants INTEGER NOT NULL DEFAULT 16`,
		`ALTER TABLE conferences ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE paging_groups ADD COLUMN mode TEXT NOT NULL DEFAULT 'unicast'`,
		`ALTER TABLE paging_groups ADD COLUMN multicast_address TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE paging_groups ADD COLUMN channel INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE paging_groups ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`,
	}
	for _, q := range alters {
		_, _ = s.db.Exec(q)
	}
	return nil
}

func (s *Store) seedDefaultAdmin() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM web_users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := HashPassword("admin")
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO web_users(username, password_hash, role, created_at) VALUES(?,?,?,?)`,
		"admin", hash, "admin", time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) SyncWebUsers(users []struct {
	Username string
	Password string
	Role     string
}) error {
	for _, u := range users {
		hash, err := HashPassword(u.Password)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(`INSERT INTO web_users(username, password_hash, role, created_at) VALUES(?,?,?,?)
			ON CONFLICT(username) DO UPDATE SET password_hash=excluded.password_hash, role=excluded.role`,
			u.Username, hash, u.Role, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return err
		}
	}
	return nil
}

// Web users
func (s *Store) ListWebUsers() ([]WebUser, error) {
	rows, err := s.db.Query(`SELECT id, username, password_hash, role FROM web_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []WebUser
	for rows.Next() {
		var u WebUser
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role); err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

func (s *Store) GetWebUserByUsername(username string) (*WebUser, error) {
	var u WebUser
	err := s.db.QueryRow(`SELECT id, username, password_hash, role FROM web_users WHERE username=?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) UpsertWebUser(username, passwordHash, role string) error {
	_, err := s.db.Exec(`INSERT INTO web_users(username, password_hash, role, created_at) VALUES(?,?,?,?)
		ON CONFLICT(username) DO UPDATE SET password_hash=excluded.password_hash, role=excluded.role`,
		username, passwordHash, role, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) DeleteWebUser(id int64) error {
	_, err := s.db.Exec(`DELETE FROM web_users WHERE id=?`, id)
	return err
}

// Hunt
func (s *Store) ListHuntGroups() ([]HuntGroup, error) {
	rows, err := s.db.Query(`SELECT id, name, number, strategy, ring_timeout_seconds, enabled FROM hunt_groups ORDER BY number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []HuntGroup
	for rows.Next() {
		var g HuntGroup
		var en int
		if err := rows.Scan(&g.ID, &g.Name, &g.Number, &g.Strategy, &g.RingTimeoutSeconds, &en); err != nil {
			return nil, err
		}
		g.Enabled = en != 0
		list = append(list, g)
	}
	return list, rows.Err()
}

func (s *Store) GetHuntGroup(id int64) (*HuntGroup, error) {
	var g HuntGroup
	var en int
	err := s.db.QueryRow(`SELECT id, name, number, strategy, ring_timeout_seconds, enabled FROM hunt_groups WHERE id=?`, id).
		Scan(&g.ID, &g.Name, &g.Number, &g.Strategy, &g.RingTimeoutSeconds, &en)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.Enabled = en != 0
	return &g, nil
}

func (s *Store) GetHuntGroupByNumber(number string) (*HuntGroup, error) {
	var g HuntGroup
	var en int
	err := s.db.QueryRow(`SELECT id, name, number, strategy, ring_timeout_seconds, enabled FROM hunt_groups WHERE number=?`, number).
		Scan(&g.ID, &g.Name, &g.Number, &g.Strategy, &g.RingTimeoutSeconds, &en)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.Enabled = en != 0
	return &g, nil
}

func (s *Store) HuntMembers(groupID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT extension FROM hunt_group_members WHERE group_id=? ORDER BY priority, id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var exts []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		exts = append(exts, e)
	}
	return exts, rows.Err()
}

func (s *Store) CreateHuntGroup(name, number, strategy string, ringTimeout int) error {
	if ringTimeout <= 0 {
		ringTimeout = 20
	}
	_, err := s.db.Exec(`INSERT INTO hunt_groups(name, number, strategy, ring_timeout_seconds) VALUES(?,?,?,?)`,
		name, number, strategy, ringTimeout)
	return err
}

func (s *Store) DeleteHuntGroup(id int64) error {
	_, err := s.db.Exec(`DELETE FROM hunt_group_members WHERE group_id=?`, id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM hunt_groups WHERE id=?`, id)
	return err
}

func (s *Store) SetHuntMembers(groupID int64, members []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM hunt_group_members WHERE group_id=?`, groupID); err != nil {
		return err
	}
	for i, m := range members {
		if _, err := tx.Exec(`INSERT INTO hunt_group_members(group_id, extension, priority) VALUES(?,?,?)`, groupID, m, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) HuntGroupReferenced(number string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM trunk_routes WHERE route_type='group' AND route_target=?`, number).Scan(&n)
	return n > 0, err
}

// Conference
func (s *Store) ListConferences() ([]Conference, error) {
	rows, err := s.db.Query(`SELECT id, name, number, pin_hash, max_participants, enabled FROM conferences ORDER BY number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Conference
	for rows.Next() {
		var c Conference
		var en int
		if err := rows.Scan(&c.ID, &c.Name, &c.Number, &c.PINHash, &c.MaxParticipants, &en); err != nil {
			return nil, err
		}
		c.Enabled = en != 0
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) GetConferenceByNumber(number string) (*Conference, error) {
	var c Conference
	var en int
	err := s.db.QueryRow(`SELECT id, name, number, pin_hash, max_participants, enabled FROM conferences WHERE number=?`, number).
		Scan(&c.ID, &c.Name, &c.Number, &c.PINHash, &c.MaxParticipants, &en)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Enabled = en != 0
	return &c, nil
}

func (s *Store) CreateConference(name, number, pin string, maxParticipants int) error {
	if maxParticipants < 2 {
		maxParticipants = 16
	}
	hash, err := HashPassword(pin)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO conferences(name, number, pin_hash, max_participants) VALUES(?,?,?,?)`,
		name, number, hash, maxParticipants)
	return err
}

func (s *Store) DeleteConference(id int64) error {
	_, err := s.db.Exec(`DELETE FROM conferences WHERE id=?`, id)
	return err
}

// Paging
func (s *Store) ListPagingGroups() ([]PagingGroup, error) {
	rows, err := s.db.Query(`SELECT id, name, code, mode, multicast_address, channel, enabled FROM paging_groups ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PagingGroup
	for rows.Next() {
		var p PagingGroup
		var en int
		if err := rows.Scan(&p.ID, &p.Name, &p.Code, &p.Mode, &p.MulticastAddress, &p.Channel, &en); err != nil {
			return nil, err
		}
		p.Enabled = en != 0
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Store) GetPagingByCode(code string) (*PagingGroup, error) {
	code = trimStar(code)
	var p PagingGroup
	var en int
	err := s.db.QueryRow(`SELECT id, name, code, mode, multicast_address, channel, enabled FROM paging_groups WHERE code=?`, code).
		Scan(&p.ID, &p.Name, &p.Code, &p.Mode, &p.MulticastAddress, &p.Channel, &en)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.Enabled = en != 0
	return &p, nil
}

func trimStar(s string) string {
	for len(s) > 0 && s[0] == '*' {
		s = s[1:]
	}
	return s
}

func (s *Store) PagingMembers(groupID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT extension FROM paging_group_members WHERE group_id=?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var exts []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		exts = append(exts, e)
	}
	return exts, rows.Err()
}

func (s *Store) CreatePagingGroup(name, code, mode, multicastAddr string, channel int) error {
	code = trimStar(code)
	if mode == "" {
		mode = "unicast"
	}
	_, err := s.db.Exec(`INSERT INTO paging_groups(name, code, mode, multicast_address, channel) VALUES(?,?,?,?,?)`,
		name, code, mode, multicastAddr, channel)
	return err
}

func (s *Store) DeletePagingGroup(id int64) error {
	_, err := s.db.Exec(`DELETE FROM paging_group_members WHERE group_id=?`, id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM paging_groups WHERE id=?`, id)
	return err
}

func (s *Store) SetPagingMembers(groupID int64, members []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM paging_group_members WHERE group_id=?`, groupID); err != nil {
		return err
	}
	for _, m := range members {
		if _, err := tx.Exec(`INSERT INTO paging_group_members(group_id, extension) VALUES(?,?)`, groupID, m); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Trunk routes (inbound only; connection details in config.toml)
func (s *Store) GetTrunkRoute(trunkID int) (*TrunkRoute, error) {
	var t TrunkRoute
	err := s.db.QueryRow(`SELECT trunk_id, route_type, route_target FROM trunk_routes WHERE trunk_id=?`, trunkID).
		Scan(&t.TrunkID, &t.RouteType, &t.RouteTarget)
	if err == sql.ErrNoRows {
		return &TrunkRoute{TrunkID: trunkID, RouteType: "all"}, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ListTrunkRoutes() ([]TrunkRoute, error) {
	rows, err := s.db.Query(`SELECT trunk_id, route_type, route_target FROM trunk_routes ORDER BY trunk_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TrunkRoute
	for rows.Next() {
		var t TrunkRoute
		if err := rows.Scan(&t.TrunkID, &t.RouteType, &t.RouteTarget); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) SaveTrunkRoute(t TrunkRoute) error {
	if t.RouteType == "" {
		t.RouteType = "all"
	}
	_, err := s.db.Exec(`INSERT INTO trunk_routes(trunk_id, route_type, route_target) VALUES(?,?,?)
		ON CONFLICT(trunk_id) DO UPDATE SET route_type=excluded.route_type, route_target=excluded.route_target`,
		t.TrunkID, t.RouteType, t.RouteTarget)
	return err
}

func FormatErr(id int, msg string) error {
	return fmt.Errorf("trunk %d: %s", id, msg)
}
