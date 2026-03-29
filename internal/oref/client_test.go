package oref_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/mbrg/chill/internal/oref"
)

const testAddr = "127.0.0.1:18080"

func startTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	l, err := net.Listen("tcp", testAddr)
	if err != nil {
		t.Fatalf("listen %s: %v", testAddr, err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = l
	srv.Start()
	return srv
}

func serveFixture(t *testing.T, fixturePath string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", fixturePath, err)
	}
	return startTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(data)
	}))
}

func TestFetchAlerts_Rocket(t *testing.T) {
	srv := serveFixture(t, "../../testdata/alert_rocket.json")
	defer srv.Close()

	c := oref.NewClient(oref.WithBaseURL(srv.URL))
	alert, err := c.FetchAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alert == nil {
		t.Fatal("expected alert, got nil")
	}
	if alert.Cat != "1" {
		t.Errorf("cat = %q, want %q", alert.Cat, "1")
	}
	if len(alert.Data) != 2 {
		t.Fatalf("areas len = %d, want 2", len(alert.Data))
	}
	if alert.Data[0] != "נתיבות" {
		t.Errorf("area[0] = %q, want %q", alert.Data[0], "נתיבות")
	}
	if alert.Data[1] != "תקומה" {
		t.Errorf("area[1] = %q, want %q", alert.Data[1], "תקומה")
	}
}

func TestFetchAlerts_Empty(t *testing.T) {
	srv := serveFixture(t, "../../testdata/alerts_empty.bin")
	defer srv.Close()

	c := oref.NewClient(oref.WithBaseURL(srv.URL))
	alert, err := c.FetchAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alert != nil {
		t.Errorf("expected nil for empty response, got %+v", alert)
	}
}

func TestFetchAlerts_Malformed(t *testing.T) {
	srv := startTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	c := oref.NewClient(oref.WithBaseURL(srv.URL))
	_, err := c.FetchAlerts(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestFetchAlerts_BOMPrefix(t *testing.T) {
	// Prepend UTF-8 BOM to valid JSON
	data, err := os.ReadFile("../../testdata/alert_rocket.json")
	if err != nil {
		t.Fatal(err)
	}
	bom := []byte{0xef, 0xbb, 0xbf}
	withBOM := append(bom, data...)

	srv := startTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(withBOM)
	}))
	defer srv.Close()

	c := oref.NewClient(oref.WithBaseURL(srv.URL))
	alert, err := c.FetchAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alert == nil {
		t.Fatal("expected alert, got nil")
	}
	if alert.Cat != "1" {
		t.Errorf("cat = %q, want %q", alert.Cat, "1")
	}
}

func TestFetchAlerts_ChecksHeaders(t *testing.T) {
	var gotReferer, gotXRW, gotAccept string
	srv := startTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("Referer")
		gotXRW = r.Header.Get("X-Requested-With")
		gotAccept = r.Header.Get("Accept")
		w.Write([]byte{0xef, 0xbb, 0xbf, 0x0d, 0x0a}) // empty BOM response
	}))
	defer srv.Close()

	c := oref.NewClient(oref.WithBaseURL(srv.URL))
	c.FetchAlerts(context.Background())

	if gotReferer != "https://www.oref.org.il/" {
		t.Errorf("Referer = %q, want %q", gotReferer, "https://www.oref.org.il/")
	}
	if gotXRW != "XMLHttpRequest" {
		t.Errorf("X-Requested-With = %q, want %q", gotXRW, "XMLHttpRequest")
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want %q", gotAccept, "application/json")
	}
}

func TestClassifyCategory(t *testing.T) {
	tests := []struct {
		cat  int
		want oref.EventType
	}{
		{1, oref.EventAlert},       // missilealert
		{2, oref.EventAlert},       // uav
		{3, oref.EventAlert},       // nonconventional
		{4, oref.EventAlert},       // warning
		{7, oref.EventAlert},       // earthquakealert1
		{9, oref.EventAlert},       // cbrne
		{10, oref.EventAlert},      // terrorattack
		{11, oref.EventAlert},      // tsunami
		{12, oref.EventAlert},      // hazmat
		{13, oref.EventEndOfEvent}, // update
		{14, oref.EventPreAlert},   // flash
		{15, oref.EventDrill},      // missilealertdrill
		{20, oref.EventDrill},      // mid-range drill
		{28, oref.EventDrill},      // flashdrill
		{0, oref.EventUnknown},     // out of range
		{29, oref.EventUnknown},    // out of range
		{-1, oref.EventUnknown},    // negative
		{100, oref.EventUnknown},   // way out of range
	}
	for _, tt := range tests {
		got := oref.ClassifyCategory(tt.cat)
		if got != tt.want {
			t.Errorf("ClassifyCategory(%d) = %v, want %v", tt.cat, got, tt.want)
		}
	}
}

func TestAlertResponse_ToEvent(t *testing.T) {
	resp := &oref.AlertResponse{
		ID:    "133456789012345678",
		Cat:   "1",
		Title: "ירי רקטות וטילים",
		Data:  []string{"נתיבות", "תקומה"},
		Desc:  "היכנסו למרחב המוגן ושהו בו 10 דקות",
	}
	ev := resp.ToEvent()
	if ev.ID != resp.ID {
		t.Errorf("ID = %q, want %q", ev.ID, resp.ID)
	}
	if ev.Category != 1 {
		t.Errorf("Category = %d, want 1", ev.Category)
	}
	if ev.EventType != oref.EventAlert {
		t.Errorf("EventType = %v, want Alert", ev.EventType)
	}
	if len(ev.Areas) != 2 {
		t.Fatalf("Areas len = %d, want 2", len(ev.Areas))
	}
	if ev.Areas[0] != "נתיבות" {
		t.Errorf("Areas[0] = %q, want %q", ev.Areas[0], "נתיבות")
	}

	// End-of-event
	endResp := &oref.AlertResponse{ID: "1", Cat: "13", Title: "האירוע הסתיים", Data: []string{"נתיבות"}}
	endEv := endResp.ToEvent()
	if endEv.EventType != oref.EventEndOfEvent {
		t.Errorf("EventType = %v, want EndOfEvent", endEv.EventType)
	}

	// Drill
	drillResp := &oref.AlertResponse{ID: "1", Cat: "15", Title: "תרגיל"}
	drillEv := drillResp.ToEvent()
	if drillEv.EventType != oref.EventDrill {
		t.Errorf("EventType = %v, want Drill", drillEv.EventType)
	}

	// Invalid cat string
	badResp := &oref.AlertResponse{ID: "1", Cat: "abc"}
	badEv := badResp.ToEvent()
	if badEv.Category != 0 {
		t.Errorf("Category = %d, want 0 for unparseable cat", badEv.Category)
	}
	if badEv.EventType != oref.EventUnknown {
		t.Errorf("EventType = %v, want Unknown", badEv.EventType)
	}
}

func TestFetchDistricts(t *testing.T) {
	srv := serveFixture(t, "../../testdata/districts.json")
	defer srv.Close()

	c := oref.NewClient(oref.WithBaseURL(srv.URL))
	districts, err := c.FetchDistricts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(districts) < 1000 {
		t.Fatalf("districts len = %d, want > 1000", len(districts))
	}
	// Spot-check first entry
	d := districts[0]
	if d.Label == "" {
		t.Error("first district label is empty")
	}
	if d.AreaName == "" {
		t.Error("first district areaname is empty")
	}
	if d.MigunTime <= 0 {
		t.Errorf("first district migun_time = %d, want > 0", d.MigunTime)
	}
}

func TestFetchCategories(t *testing.T) {
	srv := serveFixture(t, "../../testdata/categories.json")
	defer srv.Close()

	c := oref.NewClient(oref.WithBaseURL(srv.URL))
	categories, err := c.FetchCategories(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(categories) == 0 {
		t.Fatal("expected categories, got empty")
	}

	// Check cat 13 = "update", cat 14 = "flash"
	catMap := make(map[int]string)
	for _, c := range categories {
		catMap[c.ID] = c.Category
	}
	if catMap[13] != "update" {
		t.Errorf("cat 13 = %q, want %q", catMap[13], "update")
	}
	if catMap[14] != "flash" {
		t.Errorf("cat 14 = %q, want %q", catMap[14], "flash")
	}
	if catMap[1] != "missilealert" {
		t.Errorf("cat 1 = %q, want %q", catMap[1], "missilealert")
	}
}
