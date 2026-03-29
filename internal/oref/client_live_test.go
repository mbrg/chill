//go:build live

package oref_test

import (
	"context"
	"testing"

	"github.com/mbrg/chill/internal/oref"
)

func TestLive_FetchAlerts(t *testing.T) {
	c := oref.NewClient()
	alert, err := c.FetchAlerts(context.Background())
	if err != nil {
		t.Fatalf("FetchAlerts: %v", err)
	}
	// alert may be nil (no active alert) — that's fine
	if alert != nil {
		if alert.Cat == "" {
			t.Error("alert.Cat is empty")
		}
		if len(alert.Data) == 0 {
			t.Error("alert.Data is empty")
		}
	}
	t.Logf("alert = %+v", alert)
}

func TestLive_FetchDistricts(t *testing.T) {
	c := oref.NewClient()
	districts, err := c.FetchDistricts(context.Background())
	if err != nil {
		t.Fatalf("FetchDistricts: %v", err)
	}
	if len(districts) < 1000 {
		t.Fatalf("districts len = %d, want > 1000", len(districts))
	}
	t.Logf("loaded %d districts", len(districts))
}

func TestLive_FetchCategories(t *testing.T) {
	c := oref.NewClient()
	categories, err := c.FetchCategories(context.Background())
	if err != nil {
		t.Fatalf("FetchCategories: %v", err)
	}
	if len(categories) == 0 {
		t.Fatal("no categories returned")
	}
	t.Logf("loaded %d categories", len(categories))
}
