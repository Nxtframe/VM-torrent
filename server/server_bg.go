package server

import (
	"strings"
	"sync/atomic"
	"time"
)

func (s *Server) backgroundRoutines() {

	go s.fetchSearchConfig(s.engineConfig.ScraperURL) // nolint: errcheck

	// initial state
	s.state.Stats.System.loadStats()
	//collecting sys stats
	go func() {
		for {
			select {
			case <-s.syncConnected:
				if atomic.CompareAndSwapInt32(&(s.syncSemphor), 0, 1) {
					go s.tickerRoutine()
				}
			case <-s.engine.TsChanged: // task added/deleted
				s.engine.RLock()
				s.state.Push()
				s.engine.RUnlock()
			}
		}
	}()

	// rss updater
	go func() {
		// skip if not configured
		if strings.TrimSpace(s.engineConfig.RssURL) == "" {
			return
		}

		s.updateRSS()
		tk := time.NewTicker(30 * time.Minute)
		defer tk.Stop()
		for range tk.C {
			s.updateRSS()
		}
	}()

	go s.engine.RestoreCacheDir()
	if err := s.engine.StartTorrentWatcher(); err != nil {
		log.Println(err)
	}
}

// stateRoutines watches the tasks / sys states
func (s *Server) tickerRoutine() {
	defer atomic.StoreInt32(&(s.syncSemphor), 0)

	tick := time.Duration(s.IntevalSec) * time.Second
	log.Println("[tickerRoutine] sync connected, ticking for", tick)
	tk := time.NewTicker(tick)
	defer tk.Stop()

	// Save stats every 5 minutes
	saveInterval := time.NewTicker(5 * time.Minute)
	defer saveInterval.Stop()

	done := make(chan struct{})
	go func() {
		s.syncWg.Wait()
		close(done)
	}()

	for {
		select {
		case <-tk.C:
			s.state.Stats.System.loadStats()
			s.state.Stats.ConnStat = s.engine.ConnStat()
			s.updateTransferStats()
			s.engine.RLock()
			s.state.Push()
			s.engine.RUnlock()
		case <-saveInterval.C:
			s.saveTransferStats()
		case <-done:
			log.Println("[tickerRoutine] sync exit")
			s.saveTransferStats() // Save on exit
			return
		}
	}
}

// updateTransferStats updates the total transfer stats from current torrent activity
func (s *Server) updateTransferStats() {
	torrents := s.engine.GetTorrents()
	if torrents == nil {
		return
	}

	var totalDownloaded int64
	var totalUploaded int64

	s.engine.RLock()
	for _, t := range *torrents {
		if t == nil {
			continue
		}
		totalDownloaded += t.Downloaded
		totalUploaded += t.Uploaded
	}
	s.engine.RUnlock()

	downloadDelta := totalDownloaded - s.lastTotalDownloaded
	uploadDelta := totalUploaded - s.lastTotalUploaded

	if downloadDelta > 0 {
		s.state.Stats.TransferStat.TotalDownloadedBytes += downloadDelta
	}
	if uploadDelta > 0 {
		s.state.Stats.TransferStat.TotalUploadedBytes += uploadDelta
	}

	s.lastTotalDownloaded = totalDownloaded
	s.lastTotalUploaded = totalUploaded
}

// saveTransferStats saves the transfer stats to the stats file
func (s *Server) saveTransferStats() {
	if s.statsFilePath == "" {
		return
	}

	err := SaveTransferStats(&s.state.Stats.TransferStat, s.statsFilePath)
	if err != nil {
		log.Printf("[server] Failed to save transfer stats: %v", err)
	}
}
