package server

import (
 	"bufio"
 	"crypto/sha256"
 	"encoding/hex"
 	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
 	"strconv"
	"strings"
	"sync"
	"time"
)

type fileProbeInfo struct {
	VideoCodec string `json:"videoCodec"`
	AudioCodec string `json:"audioCodec"`
	Duration   float64 `json:"duration"`
	CanCopy    bool   `json:"canCopy"`
}

type convertJob struct {
	state    string
	progress float64
	err      string
	cacheRel string
	updated  time.Time
}

var convertJobs sync.Map

// serveTranscode streams video transcoded via FFmpeg to MP4
func (s *Server) serveTranscode(w http.ResponseWriter, r *http.Request) {
	dldir := s.engineConfig.DownloadDirectory
	// URL decode path for Japanese/special characters
	decodedPath, err := url.PathUnescape(r.URL.Path)
	if err != nil {
		decodedPath = r.URL.Path
	}
	filePath, err := filepath.Abs(filepath.Join(dldir, decodedPath))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Security check
	if !strings.HasPrefix(filePath, dldir) || dldir == filePath {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Check if FFmpeg is available
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		http.Error(w, "Transcoding not available - FFmpeg not found", http.StatusServiceUnavailable)
		return
	}

	// Detect hardware encoder
	hwEncoder := detectHardwareEncoder(ffmpegPath)

	// Set headers for MP4 streaming
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Cache-Control", "no-cache")

	// FFmpeg args with hardware encoding if available
	args := buildTranscodeArgs(filePath, hwEncoder)

	cmd := exec.Command(ffmpegPath, args...)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Client may have disconnected, log but don't error
		return
	}
}

func (s *Server) probeFileCodecs(filePath string) (*fileProbeInfo, error) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, err
	}

	// Get video codec
	videoCmd := exec.Command(ffprobePath, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name", "-of", "default=nk=1:nw=1", filePath)
	videoOut, _ := videoCmd.Output()
	videoCodec := strings.TrimSpace(string(videoOut))

	// Get audio codec
	audioCmd := exec.Command(ffprobePath, "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=codec_name", "-of", "default=nk=1:nw=1", filePath)
	audioOut, _ := audioCmd.Output()
	audioCodec := strings.TrimSpace(string(audioOut))

	// Get duration
	duration, _ := probeDurationSeconds(filePath)

	// Check if codecs can be copied to MP4
	// MP4 supports: h264, hevc, av1, mpeg4 (video) + aac, mp3, opus (audio)
	copyVideo := videoCodec == "h264" || videoCodec == "hevc" || videoCodec == "av1" || videoCodec == "mpeg4"
	copyAudio := audioCodec == "aac" || audioCodec == "mp3" || audioCodec == "opus"

	return &fileProbeInfo{
		VideoCodec: videoCodec,
		AudioCodec: audioCodec,
		Duration:   duration,
		CanCopy:    copyVideo && copyAudio,
	}, nil
}

func (s *Server) convertStatus(relPath string) (*convertJob, error) {
	filePath, dldir, err := s.safeAbsDownloadPath(relPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("file not found")
	}

	cacheRel, cacheAbs := s.convertCachePaths(dldir, filePath, info)
	if st, err := os.Stat(cacheAbs); err == nil && st.Size() > 0 {
		return &convertJob{state: "done", progress: 1, cacheRel: cacheRel, updated: time.Now()}, nil
	}
	// If partial file exists, treat as running
	if _, err := os.Stat(cacheAbs + ".tmp.mp4"); err == nil {
		if v, ok := convertJobs.Load(cacheRel); ok {
			j := v.(*convertJob)
			cp := *j
			cp.state = "running"
			cp.cacheRel = cacheRel
			return &cp, nil
		}
		return &convertJob{state: "running", progress: 0, cacheRel: cacheRel, updated: time.Now()}, nil
	}

	key := cacheRel
	if v, ok := convertJobs.Load(key); ok {
		j := v.(*convertJob)
		cp := *j
		cp.cacheRel = cacheRel
		return &cp, nil
	}
	return &convertJob{state: "idle", progress: 0, cacheRel: cacheRel, updated: time.Now()}, nil
}

func (s *Server) startConvert(relPath string) (*convertJob, error) {
	filePath, dldir, err := s.safeAbsDownloadPath(relPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("file not found")
	}

	cacheRel, cacheAbs := s.convertCachePaths(dldir, filePath, info)
	if st, err := os.Stat(cacheAbs); err == nil && st.Size() > 0 {
		return &convertJob{state: "done", progress: 1, cacheRel: cacheRel, updated: time.Now()}, nil
	}
	if _, err := os.Stat(cacheAbs + ".tmp.mp4"); err == nil {
		key := cacheRel
		if v, ok := convertJobs.Load(key); ok {
			j := v.(*convertJob)
			cp := *j
			cp.state = "running"
			cp.cacheRel = cacheRel
			return &cp, nil
		}
		return &convertJob{state: "running", progress: 0, cacheRel: cacheRel, updated: time.Now()}, nil
	}

	key := cacheRel
	if v, ok := convertJobs.Load(key); ok {
		j := v.(*convertJob)
		cp := *j
		cp.cacheRel = cacheRel
		return &cp, nil
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found")
	}
	_ = os.MkdirAll(filepath.Join(dldir, ".transcode_cache"), 0755)

	j := &convertJob{state: "running", progress: 0, cacheRel: cacheRel, updated: time.Now()}
	convertJobs.Store(key, j)

	go func() {
		err := s.runConvert(ffmpegPath, filePath, cacheAbs, key)
		if err != nil {
			errMsg := s.formatConvertError(err)
			updateConvertJob(key, "error", 0, errMsg)
			return
		}
		updateConvertJob(key, "done", 1, "")
	}()

	cp := *j
	cp.cacheRel = cacheRel
	return &cp, nil
}

func updateConvertJob(key, state string, progress float64, errStr string) {
	v, ok := convertJobs.Load(key)
	if !ok {
		return
	}
	j := v.(*convertJob)
	j.state = state
	if progress >= 0 {
		j.progress = progress
	}
	j.err = errStr
	j.updated = time.Now()
	if state == "done" || state == "error" {
		// keep it for refresh reads while server is running
	}
}

func (s *Server) formatConvertError(err error) string {
	errStr := err.Error()
	// Translate common FFmpeg exit codes to readable messages
	if strings.Contains(errStr, "0xffffffea") || strings.Contains(errStr, "exit status 234") {
		return "FFmpeg error: Invalid argument (file may be corrupted or unsupported)"
	}
	if strings.Contains(errStr, "0x1") || strings.Contains(errStr, "exit status 1") {
		return "FFmpeg error: General error occurred"
	}
	if strings.Contains(errStr, "broken pipe") || strings.Contains(errStr, "Broken pipe") {
		return "Conversion interrupted"
	}
	if strings.Contains(errStr, "No such file") {
		return "Source file not found"
	}
	if strings.Contains(errStr, "Permission denied") {
		return "Permission denied - check cache directory access"
	}
	// Return original if no translation
	return errStr
}

func (s *Server) runConvert(ffmpegPath, inputAbs, outputAbs, key string) error {
	duration, _ := probeDurationSeconds(inputAbs)
	tmpAbs := outputAbs + ".tmp.mp4"
	_ = os.Remove(tmpAbs)

	// Try copy first if codecs are compatible
	args := []string{"-y", "-fflags", "+genpts", "-err_detect", "ignore_err", "-i", inputAbs, "-c:v", "copy", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", "-progress", "pipe:1", "-nostats", tmpAbs}
	err := runFFmpegWithProgress(ffmpegPath, args, duration, func(p float64) {
		updateConvertJob(key, "running", p, "")
	})
	if err == nil {
		_ = os.Remove(outputAbs)
		if renErr := os.Rename(tmpAbs, outputAbs); renErr != nil {
			_ = os.Remove(tmpAbs)
			return renErr
		}
		return nil
	}

	_ = os.Remove(tmpAbs)
	// Fallback: re-encode both video and audio
	args = []string{"-y", "-fflags", "+genpts", "-err_detect", "ignore_err", "-i", inputAbs, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", "-progress", "pipe:1", "-nostats", tmpAbs}
	err2 := runFFmpegWithProgress(ffmpegPath, args, duration, func(p float64) {
		updateConvertJob(key, "running", p, "")
	})
	if err2 == nil {
		_ = os.Remove(outputAbs)
		if renErr := os.Rename(tmpAbs, outputAbs); renErr != nil {
			_ = os.Remove(tmpAbs)
			return renErr
		}
		return nil
	}
	_ = os.Remove(tmpAbs)
	return err
}

func probeDurationSeconds(inputAbs string) (float64, error) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(ffprobePath, "-v", "error", "-show_entries", "format=duration", "-of", "default=nk=1:nw=1", inputAbs)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, errors.New("empty duration")
	}
	return strconv.ParseFloat(s, 64)
}

func runFFmpegWithProgress(ffmpegPath string, args []string, duration float64, onProgress func(float64)) error {
	cmd := exec.Command(ffmpegPath, args...)
	// -progress pipe:1 outputs to stdout, stderr has errors only
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr // Keep stderr for debugging
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	var currentTimeMs float64
	for scanner.Scan() {
		line := scanner.Text()
		// Parse -progress output format: out_time_us=12345678 (microseconds) or out_time=12.345678 (seconds)
		if strings.HasPrefix(line, "out_time_ms=") {
			// out_time_ms is actually microseconds in FFmpeg
			usStr := strings.TrimPrefix(line, "out_time_ms=")
			if us, err := strconv.ParseFloat(usStr, 64); err == nil {
				currentTimeMs = us / 1000 // Convert microseconds to milliseconds
			}
		} else if strings.HasPrefix(line, "out_time=") {
			timeStr := strings.TrimPrefix(line, "out_time=")
			if sec, err := strconv.ParseFloat(timeStr, 64); err == nil {
				currentTimeMs = sec * 1000
			}
		}
		// progress=end means finished
		if line == "progress=end" {
			onProgress(1.0)
		}
		// Calculate progress whenever we have time and duration
		if duration > 0 && currentTimeMs > 0 {
			p := (currentTimeMs / 1000) / duration
			if p < 0 {
				p = 0
			}
			if p > 1.0 {
				p = 1.0
			}
			onProgress(p)
		}
	}
	_ = stdout.Close()
	_ = scanner.Err()
	return cmd.Wait()
}

func parseFFmpegTimeSeconds(line string) (float64, bool) {
	idx := strings.Index(line, "time=")
	if idx < 0 {
		return 0, false
	}
	s := line[idx+5:]
	end := strings.IndexAny(s, " \r\n")
	if end >= 0 {
		s = s[:end]
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.ParseFloat(parts[0], 64)
	m, err2 := strconv.ParseFloat(parts[1], 64)
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return h*3600 + m*60 + sec, true
}

func (s *Server) safeAbsDownloadPath(relPath string) (abs string, dldir string, err error) {
	dldir = s.engineConfig.DownloadDirectory
	// URL decode for Japanese/special characters
	decodedPath, decErr := url.PathUnescape(relPath)
	if decErr == nil {
		relPath = decodedPath
	}
	abs, err = filepath.Abs(filepath.Join(dldir, relPath))
	if err != nil {
		return "", dldir, err
	}
	if !strings.HasPrefix(abs, dldir) || dldir == abs {
		return "", dldir, fmt.Errorf("invalid path")
	}
	return abs, dldir, nil
}

func (s *Server) convertCachePaths(dldir, inputAbs string, info os.FileInfo) (cacheRel string, cacheAbs string) {
	// Use stable hash based on file path and size only (not ModTime)
	// This ensures same file always gets same cache name even if converting multiple times
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", inputAbs, info.Size())))
	name := hex.EncodeToString(h[:16]) + ".mp4"
	cacheRel = filepath.ToSlash(filepath.Join(".transcode_cache", name))
	cacheAbs = filepath.Join(dldir, ".transcode_cache", name)
	return cacheRel, cacheAbs
}

// detectHardwareEncoder checks for available hardware encoders
func detectHardwareEncoder(ffmpegPath string) string {
	// Try Intel QuickSync (most common, fastest)
	if err := exec.Command(ffmpegPath, "-encoders").Wait(); err == nil {
		out, _ := exec.Command(ffmpegPath, "-encoders").Output()
		if strings.Contains(string(out), "h264_qsv") {
			return "h264_qsv"
		}
		if strings.Contains(string(out), "h264_nvenc") {
			return "h264_nvenc"
		}
		if strings.Contains(string(out), "h264_amf") {
			return "h264_amf"
		}
	}
	return "libx264" // Software fallback
}

// buildTranscodeArgs creates FFmpeg args based on encoder
func buildTranscodeArgs(filePath, encoder string) []string {
	baseArgs := []string{
		"-hwaccel", "auto",
		"-i", filePath,
	}

	switch encoder {
	case "h264_qsv": // Intel QuickSync - fastest, good quality
		return append(baseArgs,
			"-c:v", "h264_qsv",
			"-preset", "medium",
			"-global_quality", "23",
			"-look_ahead", "0",
			"-c:a", "aac",
			"-b:a", "128k",
			"-movflags", "frag_keyframe+empty_moov+faststart",
			"-f", "mp4",
			"pipe:1",
		)
	case "h264_nvenc": // NVIDIA NVENC
		return append(baseArgs,
			"-c:v", "h264_nvenc",
			"-preset", "p4",
			"-cq", "23",
			"-c:a", "aac",
			"-b:a", "128k",
			"-movflags", "frag_keyframe+empty_moov+faststart",
			"-f", "mp4",
			"pipe:1",
		)
	case "h264_amf": // AMD VCE
		return append(baseArgs,
			"-c:v", "h264_amf",
			"-quality", "speed",
			"-qp_i", "23",
			"-qp_p", "23",
			"-c:a", "aac",
			"-b:a", "128k",
			"-movflags", "frag_keyframe+empty_moov+faststart",
			"-f", "mp4",
			"pipe:1",
		)
	default: // Software fallback - good quality
		return append(baseArgs,
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-crf", "23",
			"-c:a", "aac",
			"-b:a", "128k",
			"-movflags", "frag_keyframe+empty_moov+faststart",
			"-f", "mp4",
			"pipe:1",
		)
	}
}

// needsTranscoding checks if file format needs server-side transcoding
func needsTranscoding(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".ts", ".mpeg", ".mpg", ".mkv", ".avi", ".mov":
		return true
	default:
		return false
	}
}
