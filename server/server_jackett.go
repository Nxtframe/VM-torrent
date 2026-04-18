package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

// JackettClient handles communication with Jackett API
type JackettClient struct {
	BaseURL string // e.g., "http://localhost:9117"
	APIKey  string
	Client  *http.Client
}

// JackettResult represents a single torrent result
type JackettResult struct {
	Title       string   `json:"Title"`
	MagnetUri   string   `json:"MagnetUri"`
	Link        string   `json:"Link"` // Alternative download link
	Seeders     int      `json:"Seeders"`
	Peers       int      `json:"Peers"`
	Size        int64    `json:"Size"`
	Category    []int    `json:"Category"`
	PublishDate string   `json:"PublishDate"`
}

// JackettResponse is the API response structure
type JackettResponse struct {
	Results []JackettResult `json:"Results"`
}

// NewJackettClient creates a new Jackett client
func NewJackettClient(baseURL, apiKey string) *JackettClient {
	return &JackettClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client:  &http.Client{},
	}
}

// Search searches across all configured Jackett indexers
func (j *JackettClient) Search(query string) ([]JackettResult, error) {
	return j.SearchWithIndexers(query, "all")
}

// SearchWithIndexers searches specific indexers (comma-separated, e.g., "1337x,yts" or "all")
func (j *JackettClient) SearchWithIndexers(query, indexers string) ([]JackettResult, error) {
	apiURL := fmt.Sprintf("%s/api/v2.0/indexers/%s/results", j.BaseURL, indexers)

	params := url.Values{}
	params.Set("apikey", j.APIKey)
	params.Set("Query", query)

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	resp, err := j.Client.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("jackett request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("jackett returned %d: %s", resp.StatusCode, string(body))
	}

	var result JackettResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode jackett response: %w", err)
	}

	return result.Results, nil
}

// SearchIndexer searches a specific indexer only
func (j *JackettClient) SearchIndexer(query, indexer string) ([]JackettResult, error) {
	return j.SearchWithIndexers(query, indexer)
}

// GetIndexerList returns list of configured indexers
func (j *JackettClient) GetIndexerList() ([]string, error) {
	apiURL := fmt.Sprintf("%s/api/v2.0/indexers?configured=true", j.BaseURL)

	params := url.Values{}
	params.Set("apikey", j.APIKey)

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	resp, err := j.Client.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("jackett request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jackett returned %d", resp.StatusCode)
	}

	var indexers []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&indexers); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(indexers))
	for _, idx := range indexers {
		if id, ok := idx["id"].(string); ok {
			names = append(names, id)
		}
	}

	return names, nil
}

// ToScraperFormat converts Jackett results to the scraper format used by the frontend
type ScraperResult struct {
	Title    string `json:"title"`
	Magnet   string `json:"magnet"`
	Seeders  int    `json:"seeders"`
	Peers    int    `json:"peers"`
	Size     string `json:"size"`
	Provider string `json:"provider"`
}

// ConvertToScraperResults converts Jackett results to frontend format
func ConvertToScraperResults(results []JackettResult, provider string) []ScraperResult {
	scraped := make([]ScraperResult, 0, len(results))
	for _, r := range results {
		magnet := r.MagnetUri
		if magnet == "" && r.Link != "" {
			magnet = r.Link
		}
		scraped = append(scraped, ScraperResult{
			Title:    r.Title,
			Magnet:   magnet,
			Seeders:  r.Seeders,
			Peers:    r.Peers,
			Size:     humanizeSize(r.Size),
			Provider: provider,
		})
	}
	return scraped
}

func humanizeSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
