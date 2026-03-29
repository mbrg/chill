package oref

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

const defaultBaseURL = "https://www.oref.org.il"

// EventType classifies an alert's lifecycle stage.
type EventType int

const (
	EventUnknown    EventType = iota
	EventAlert                // active threat (cat 1-4, 7-12)
	EventEndOfEvent           // all-clear (cat 13)
	EventPreAlert             // heads-up (cat 14)
	EventDrill                // drill (cat 15-28)
)

func (e EventType) String() string {
	switch e {
	case EventAlert:
		return "Alert"
	case EventEndOfEvent:
		return "EndOfEvent"
	case EventPreAlert:
		return "PreAlert"
	case EventDrill:
		return "Drill"
	default:
		return "Unknown"
	}
}

// ClassifyCategory maps an Oref category ID to an EventType.
func ClassifyCategory(cat int) EventType {
	switch {
	case cat >= 1 && cat <= 12:
		return EventAlert
	case cat == 13:
		return EventEndOfEvent
	case cat == 14:
		return EventPreAlert
	case cat >= 15 && cat <= 28:
		return EventDrill
	default:
		return EventUnknown
	}
}

// Event is the canonical parsed representation of an Oref alert.
type Event struct {
	ID        string
	Category  int
	EventType EventType
	Title     string
	Areas     []string
	Desc      string
}

// AlertResponse represents a real-time alert from Oref's alerts.json endpoint.
type AlertResponse struct {
	ID    string   `json:"id"`
	Cat   string   `json:"cat"`
	Title string   `json:"title"`
	Data  []string `json:"data"`
	Desc  string   `json:"desc"`
}

// ToEvent converts an AlertResponse into a canonical Event.
func (a *AlertResponse) ToEvent() Event {
	cat, _ := strconv.Atoi(a.Cat)
	return Event{
		ID:        a.ID,
		Category:  cat,
		EventType: ClassifyCategory(cat),
		Title:     a.Title,
		Areas:     a.Data,
		Desc:      a.Desc,
	}
}

// District represents a location entry from Oref's districts catalog.
type District struct {
	Label    string `json:"label_he"`
	Value    string `json:"value"`
	ID       string `json:"id"`
	AreaID   int    `json:"areaid"`
	AreaName string `json:"areaname"`
	MigunTime int   `json:"migun_time"`
}

// Category represents an alert category from Oref's categories endpoint.
type Category struct {
	ID       int    `json:"id"`
	Category string `json:"category"`
	MatrixID int    `json:"matrix_id"`
	Priority int    `json:"priority"`
}

// Client fetches data from Oref endpoints.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the Oref base URL (for testing).
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// NewClient creates an Oref client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    defaultBaseURL,
		httpClient: http.DefaultClient,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) doGet(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://www.oref.org.il/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oref: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// stripBOM removes a UTF-8 BOM prefix if present.
func stripBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, utf8BOM)
}

// FetchAlerts fetches real-time alerts. Returns nil, nil when no alert is active.
func (c *Client) FetchAlerts(ctx context.Context) (*AlertResponse, error) {
	body, err := c.doGet(ctx, "/WarningMessages/alert/alerts.json")
	if err != nil {
		return nil, err
	}

	body = stripBOM(body)
	body = bytes.TrimSpace(body)

	if len(body) == 0 {
		return nil, nil
	}

	var alert AlertResponse
	if err := json.Unmarshal(body, &alert); err != nil {
		return nil, fmt.Errorf("oref: parsing alerts: %w", err)
	}
	return &alert, nil
}

// FetchDistricts fetches the full district catalog.
func (c *Client) FetchDistricts(ctx context.Context) ([]District, error) {
	body, err := c.doGet(ctx, "/districts/GetDistricts.aspx")
	if err != nil {
		return nil, err
	}
	body = stripBOM(body)
	body = bytes.TrimSpace(body)

	var districts []District
	if err := json.Unmarshal(body, &districts); err != nil {
		return nil, fmt.Errorf("oref: parsing districts: %w", err)
	}
	return districts, nil
}

// FetchCategories fetches the alert category definitions.
func (c *Client) FetchCategories(ctx context.Context) ([]Category, error) {
	body, err := c.doGet(ctx, "/WarningMessages/alert/alertCategories.json")
	if err != nil {
		return nil, err
	}
	body = stripBOM(body)
	body = bytes.TrimSpace(body)

	var categories []Category
	if err := json.Unmarshal(body, &categories); err != nil {
		return nil, fmt.Errorf("oref: parsing categories: %w", err)
	}
	return categories, nil
}
