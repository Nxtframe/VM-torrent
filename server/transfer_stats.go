package server

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// TransferStats tracks total downloaded and uploaded bytes across sessions
type TransferStats struct {
	TotalDownloadedBytes int64 `json:"total_downloaded_bytes"`
	TotalUploadedBytes   int64 `json:"total_uploaded_bytes"`
}

// LoadTransferStats loads transfer stats from a JSON file
func LoadTransferStats(statsPath string) (*TransferStats, error) {
	stats := &TransferStats{
		TotalDownloadedBytes: 0,
		TotalUploadedBytes:   0,
	}

	data, err := os.ReadFile(statsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, return default stats
			return stats, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, stats); err != nil {
		return nil, err
	}

	return stats, nil
}

// SaveTransferStats saves transfer stats to a JSON file
func SaveTransferStats(stats *TransferStats, statsPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(statsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statsPath, data, 0644)
}

// ToMB converts bytes to megabytes
func (s *TransferStats) ToMB(bytes int64) float64 {
	return float64(bytes) / 1024 / 1024
}
