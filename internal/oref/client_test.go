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
