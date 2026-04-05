package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"
)

// User represents a registered Telegram user.
type User struct {
	ID         int64
	TelegramID int64
	Area       string
	Active     bool
	CreatedAt  string
	UpdatedAt  string
}

// Event represents a stored Oref alert event.
type Event struct {
	ID         int64
	OrefID     string
	Category   int
	Title      string
	Areas      []string
	Desc       string
	ReceivedAt string
}

// Delivery represents a notification delivery record.
type Delivery struct {
	ID      int64
	EventID int64
	UserID  int64
	Status  string
	SentAt  string
	Error   string
}

// Store provides persistence for users, events, and deliveries.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database and initializes the schema.
func New(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}

	// WAL mode for concurrent reads while poller writes.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: WAL: %w", err)
	}

	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

func createTables(db *sql.DB) error {
	const schema = `
	CREATE TABLE IF NOT EXISTS users (
		id          INTEGER PRIMARY KEY,
		telegram_id INTEGER UNIQUE NOT NULL,
		area        TEXT NOT NULL,
		active      INTEGER DEFAULT 1,
		created_at  TEXT DEFAULT (datetime('now')),
		updated_at  TEXT DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS events (
		id          INTEGER PRIMARY KEY,
		oref_id     TEXT NOT NULL,
		category    INTEGER NOT NULL,
		title       TEXT NOT NULL,
		areas       TEXT NOT NULL,
		description TEXT,
		received_at TEXT DEFAULT (datetime('now')),
		UNIQUE(oref_id, category)
	);

	CREATE TABLE IF NOT EXISTS deliveries (
		id       INTEGER PRIMARY KEY,
		event_id INTEGER REFERENCES events(id),
		user_id  INTEGER REFERENCES users(id),
		status   TEXT NOT NULL,
		sent_at  TEXT DEFAULT (datetime('now')),
		error    TEXT
	);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("store: schema: %w", err)
	}
	return nil
}

// UpsertUser inserts a new user or updates the area of an existing one.
func (s *Store) UpsertUser(telegramID int64, area string) error {
	_, err := s.db.Exec(`
		INSERT INTO users (telegram_id, area)
		VALUES (?, ?)
		ON CONFLICT(telegram_id) DO UPDATE SET
			area = excluded.area,
			active = 1,
			updated_at = datetime('now')
	`, telegramID, area)
	if err != nil {
		return fmt.Errorf("store: upsert user: %w", err)
	}
	return nil
}

// GetUser returns a user by Telegram ID, or nil if not found.
func (s *Store) GetUser(telegramID int64) (*User, error) {
	row := s.db.QueryRow(`
		SELECT id, telegram_id, area, active, created_at, updated_at
		FROM users WHERE telegram_id = ?
	`, telegramID)

	var u User
	var active int
	err := row.Scan(&u.ID, &u.TelegramID, &u.Area, &active, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get user: %w", err)
	}
	u.Active = active == 1
	return &u, nil
}

// DeactivateUser marks a user as inactive.
func (s *Store) DeactivateUser(telegramID int64) error {
	res, err := s.db.Exec(`
		UPDATE users SET active = 0, updated_at = datetime('now')
		WHERE telegram_id = ?
	`, telegramID)
	if err != nil {
		return fmt.Errorf("store: deactivate user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: user %d not found", telegramID)
	}
	return nil
}

// ActiveUsersByArea returns all active users subscribed to the given area.
func (s *Store) ActiveUsersByArea(area string) ([]User, error) {
	rows, err := s.db.Query(`
		SELECT id, telegram_id, area, active, created_at, updated_at
		FROM users WHERE area = ? AND active = 1
	`, area)
	if err != nil {
		return nil, fmt.Errorf("store: active users by area: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var active int
		if err := rows.Scan(&u.ID, &u.TelegramID, &u.Area, &active, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan user: %w", err)
		}
		u.Active = active == 1
		users = append(users, u)
	}
	return users, rows.Err()
}

// AllUsers returns all users (active and inactive).
func (s *Store) AllUsers() ([]User, error) {
	rows, err := s.db.Query(`
		SELECT id, telegram_id, area, active, created_at, updated_at
		FROM users ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("store: all users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var active int
		if err := rows.Scan(&u.ID, &u.TelegramID, &u.Area, &active, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan user: %w", err)
		}
		u.Active = active == 1
		users = append(users, u)
	}
	return users, rows.Err()
}

// InsertEvent stores an event. Returns true if it's new, false if deduplicated.
// Dedup key: (oref_id, category).
func (s *Store) InsertEvent(orefID string, category int, title string, areas []string, desc string) (isNew bool, eventID int64, err error) {
	areasJSON, err := json.Marshal(areas)
	if err != nil {
		return false, 0, fmt.Errorf("store: marshal areas: %w", err)
	}

	res, err := s.db.Exec(`
		INSERT INTO events (oref_id, category, title, areas, description)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(oref_id, category) DO NOTHING
	`, orefID, category, title, string(areasJSON), desc)
	if err != nil {
		return false, 0, fmt.Errorf("store: insert event: %w", err)
	}

	n, _ := res.RowsAffected()
	if n == 0 {
		// Dedup hit — fetch the existing event ID.
		row := s.db.QueryRow(`SELECT id FROM events WHERE oref_id = ? AND category = ?`, orefID, category)
		if err := row.Scan(&eventID); err != nil {
			return false, 0, fmt.Errorf("store: fetch dedup event: %w", err)
		}
		return false, eventID, nil
	}

	eventID, _ = res.LastInsertId()
	return true, eventID, nil
}

// RecentEvents returns the most recent events.
func (s *Store) RecentEvents(limit int) ([]Event, error) {
	rows, err := s.db.Query(`
		SELECT id, oref_id, category, title, areas, COALESCE(description,''), received_at
		FROM events ORDER BY id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: recent events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var areasJSON string
		if err := rows.Scan(&e.ID, &e.OrefID, &e.Category, &e.Title, &areasJSON, &e.Desc, &e.ReceivedAt); err != nil {
			return nil, fmt.Errorf("store: scan event: %w", err)
		}
		_ = json.Unmarshal([]byte(areasJSON), &e.Areas)
		events = append(events, e)
	}
	return events, rows.Err()
}

// LogDelivery records a delivery attempt.
func (s *Store) LogDelivery(eventID, userID int64, status string, errMsg string) error {
	_, err := s.db.Exec(`
		INSERT INTO deliveries (event_id, user_id, status, error)
		VALUES (?, ?, ?, ?)
	`, eventID, userID, status, errMsg)
	if err != nil {
		return fmt.Errorf("store: log delivery: %w", err)
	}
	return nil
}

// GetDeliveries returns all deliveries for an event.
func (s *Store) GetDeliveries(eventID int64) ([]Delivery, error) {
	rows, err := s.db.Query(`
		SELECT id, event_id, user_id, status, sent_at, COALESCE(error,'')
		FROM deliveries WHERE event_id = ? ORDER BY id
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: get deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.EventID, &d.UserID, &d.Status, &d.SentAt, &d.Error); err != nil {
			return nil, fmt.Errorf("store: scan delivery: %w", err)
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

