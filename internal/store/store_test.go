package store_test

import (
	"testing"

	"github.com/mbrg/chill/internal/store"
)

func mustNew(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// --- 2.1: Schema creation ---

func TestNew(t *testing.T) {
	s := mustNew(t)
	_ = s // tables created without error
}

// --- 2.2: User CRUD ---

func TestUserInsertAndGet(t *testing.T) {
	s := mustNew(t)

	if err := s.UpsertUser(123, "נתיבות"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	u, err := s.GetUser(123)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u == nil {
		t.Fatal("expected user, got nil")
	}
	if u.TelegramID != 123 {
		t.Errorf("TelegramID = %d, want 123", u.TelegramID)
	}
	if u.Area != "נתיבות" {
		t.Errorf("Area = %q, want %q", u.Area, "נתיבות")
	}
	if !u.Active {
		t.Error("expected user to be active")
	}
}

func TestUserUpsertUpdatesArea(t *testing.T) {
	s := mustNew(t)

	s.UpsertUser(123, "נתיבות")
	s.UpsertUser(123, "תקומה") // same telegram ID, new area

	u, _ := s.GetUser(123)
	if u.Area != "תקומה" {
		t.Errorf("Area = %q, want %q after upsert", u.Area, "תקומה")
	}
}

func TestUserDeactivate(t *testing.T) {
	s := mustNew(t)

	s.UpsertUser(123, "נתיבות")
	if err := s.DeactivateUser(123); err != nil {
		t.Fatalf("DeactivateUser: %v", err)
	}

	users, _ := s.ActiveUsersByArea("נתיבות")
	if len(users) != 0 {
		t.Errorf("expected 0 active users, got %d", len(users))
	}

	// Verify user still exists but is inactive.
	u, _ := s.GetUser(123)
	if u == nil {
		t.Fatal("expected user to still exist")
	}
	if u.Active {
		t.Error("expected user to be inactive")
	}
}

func TestUserDeactivateNotFound(t *testing.T) {
	s := mustNew(t)
	err := s.DeactivateUser(999)
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestActiveUsersByArea(t *testing.T) {
	s := mustNew(t)

	s.UpsertUser(100, "נתיבות")
	s.UpsertUser(200, "נתיבות")
	s.UpsertUser(300, "חיפה")
	s.DeactivateUser(200)

	users, err := s.ActiveUsersByArea("נתיבות")
	if err != nil {
		t.Fatalf("ActiveUsersByArea: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 active user in נתיבות, got %d", len(users))
	}
	if users[0].TelegramID != 100 {
		t.Errorf("TelegramID = %d, want 100", users[0].TelegramID)
	}

	// Different area.
	users, _ = s.ActiveUsersByArea("חיפה")
	if len(users) != 1 {
		t.Fatalf("expected 1 active user in חיפה, got %d", len(users))
	}
	if users[0].TelegramID != 300 {
		t.Errorf("TelegramID = %d, want 300", users[0].TelegramID)
	}
}

func TestUserGetNotFound(t *testing.T) {
	s := mustNew(t)
	u, err := s.GetUser(999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != nil {
		t.Errorf("expected nil for non-existent user, got %+v", u)
	}
}

func TestUpsertReactivatesUser(t *testing.T) {
	s := mustNew(t)

	s.UpsertUser(123, "נתיבות")
	s.DeactivateUser(123)
	s.UpsertUser(123, "תקומה") // re-upsert should reactivate

	u, _ := s.GetUser(123)
	if !u.Active {
		t.Error("expected user to be reactivated after upsert")
	}
	if u.Area != "תקומה" {
		t.Errorf("Area = %q, want %q", u.Area, "תקומה")
	}
}

// --- 2.3: Event dedup ---

func TestDedupInsertNew(t *testing.T) {
	s := mustNew(t)

	isNew, id, err := s.InsertEvent("alert-1", 1, "ירי רקטות", []string{"נתיבות", "תקומה"}, "desc")
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true for first insert")
	}
	if id == 0 {
		t.Error("expected non-zero event ID")
	}
}

func TestDedupSameEvent(t *testing.T) {
	s := mustNew(t)

	s.InsertEvent("alert-1", 1, "ירי רקטות", []string{"נתיבות"}, "")

	isNew, _, err := s.InsertEvent("alert-1", 1, "ירי רקטות", []string{"נתיבות"}, "")
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if isNew {
		t.Error("expected isNew=false for duplicate (same oref_id + category)")
	}
}

func TestDedupDifferentCategory(t *testing.T) {
	s := mustNew(t)

	s.InsertEvent("alert-1", 1, "ירי רקטות", []string{"נתיבות"}, "")

	// Same oref_id, different category (end-of-event) → should be new.
	isNew, _, err := s.InsertEvent("alert-1", 13, "האירוע הסתיים", []string{"נתיבות"}, "")
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true for same oref_id but different category")
	}
}

// --- 2.4: Delivery log ---

func TestDeliveryLog(t *testing.T) {
	s := mustNew(t)

	s.UpsertUser(123, "נתיבות")
	_, eventID, _ := s.InsertEvent("alert-1", 1, "ירי רקטות", []string{"נתיבות"}, "")
	user, _ := s.GetUser(123)

	if err := s.LogDelivery(eventID, user.ID, "sent", ""); err != nil {
		t.Fatalf("LogDelivery: %v", err)
	}

	deliveries, err := s.GetDeliveries(eventID)
	if err != nil {
		t.Fatalf("GetDeliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	d := deliveries[0]
	if d.EventID != eventID {
		t.Errorf("EventID = %d, want %d", d.EventID, eventID)
	}
	if d.UserID != user.ID {
		t.Errorf("UserID = %d, want %d", d.UserID, user.ID)
	}
	if d.Status != "sent" {
		t.Errorf("Status = %q, want %q", d.Status, "sent")
	}
	if d.Error != "" {
		t.Errorf("Error = %q, want empty", d.Error)
	}
}

func TestDeliveryLogWithError(t *testing.T) {
	s := mustNew(t)

	s.UpsertUser(123, "נתיבות")
	_, eventID, _ := s.InsertEvent("alert-1", 1, "ירי רקטות", []string{"נתיבות"}, "")
	user, _ := s.GetUser(123)

	s.LogDelivery(eventID, user.ID, "failed", "timeout")

	deliveries, _ := s.GetDeliveries(eventID)
	if deliveries[0].Status != "failed" {
		t.Errorf("Status = %q, want %q", deliveries[0].Status, "failed")
	}
	if deliveries[0].Error != "timeout" {
		t.Errorf("Error = %q, want %q", deliveries[0].Error, "timeout")
	}
}

// --- AllUsers + RecentEvents (for CLI) ---

func TestAllUsers(t *testing.T) {
	s := mustNew(t)

	s.UpsertUser(100, "נתיבות")
	s.UpsertUser(200, "חיפה")
	s.DeactivateUser(200)

	users, err := s.AllUsers()
	if err != nil {
		t.Fatalf("AllUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Active != true {
		t.Error("expected first user active")
	}
	if users[1].Active != false {
		t.Error("expected second user inactive")
	}
}

func TestRecentEvents(t *testing.T) {
	s := mustNew(t)

	s.InsertEvent("a1", 1, "Alert 1", []string{"נתיבות"}, "")
	s.InsertEvent("a2", 14, "PreAlert", []string{"חיפה"}, "")
	s.InsertEvent("a3", 13, "End", []string{"נתיבות"}, "desc")

	events, err := s.RecentEvents(10)
	if err != nil {
		t.Fatalf("RecentEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	// Most recent first.
	if events[0].OrefID != "a3" {
		t.Errorf("first event oref_id = %q, want %q", events[0].OrefID, "a3")
	}
	if len(events[0].Areas) != 1 || events[0].Areas[0] != "נתיבות" {
		t.Errorf("areas = %v, want [נתיבות]", events[0].Areas)
	}
}
