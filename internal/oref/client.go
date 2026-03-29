package oref

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

const defaultBaseURL = "https://www.oref.org.il"

// AlertResponse represents a real-time alert from Oref's alerts.json endpoint.
type AlertResponse struct {
	ID    string   `json:"id"`
	Cat   string   `json:"cat"`
	Title string   `json:"title"`
	Data  []string `json:"data"`
	Desc  string   `json:"desc"`
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
