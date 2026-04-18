package server

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// serveTranscode streams video transcoded via FFmpeg to MP4
func (s *Server) serveTranscode(w http.ResponseWriter, r *http.Request) {
	dldir := s.engineConfig.DownloadDirectory
	filePath, err := filepath.Abs(filepath.Join(dldir, r.URL.Path))
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
