package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boypt/simple-torrent/common"
	"github.com/jpillora/archive"
)

const fileNumberLimit = 65535

type fsNode struct {
	Name     string
	Path     string
	Size     int64
	Modified time.Time
	Children []*fsNode
}

func (s *Server) listFiles() *fsNode {
	rootDir := s.engineConfig.DownloadDirectory
	root := &fsNode{}
	if info, err := os.Stat(rootDir); err == nil {
		if err := list(rootDir, info, root, new(uint), ""); err != nil {
			log.Printf("File listing failed: %s", err)
		}
	}
	return root
}

func (s *Server) serveDownloadFiles(w http.ResponseWriter, r *http.Request) {
	//dldir is absolute
	dldir := s.engineConfig.DownloadDirectory
	// URL decode path so requests built with encodeURIComponent (and Unicode filenames)
	// resolve to the correct filesystem path.
	decodedPath, decErr := url.PathUnescape(r.URL.Path)
	if decErr != nil {
		decodedPath = r.URL.Path
	}
	file, err := filepath.Abs(filepath.Join(dldir, decodedPath))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	//only allow fetches/deletes inside the dl dir
	if !strings.HasPrefix(file, dldir) || dldir == file {
		http.Error(w, "Nice try\n"+dldir+"\n"+file, http.StatusBadRequest)
		return
	}

	info, err := os.Stat(file)
	if err != nil {
		http.Error(w, "File stat error: "+err.Error(), http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		if info.IsDir() {
			w.Header().Set("Content-Type", "application/zip")
			w.WriteHeader(200)
			//write .zip archive directly into response
			a := archive.NewZipWriter(w)
			common.HandleError(a.AddDir(file))
			a.Close()
		} else {
			filename := filepath.Base(file)
			asciiName := filename
			if len(asciiName) > 100 {
				hash := sha256.Sum256([]byte(file))
				ext := filepath.Ext(filename)
				asciiName = hex.EncodeToString(hash[:8]) + ext
			}
			// Support Japanese/Unicode filenames via filename*
			escaped := url.PathEscape(filename)
			w.Header().Set("Content-Disposition", `attachment; filename="`+asciiName+`"; filename*=UTF-8''`+escaped)
			http.ServeFile(w, r, file)
		}
	case "DELETE":
		if err := os.RemoveAll(file); err != nil {
			http.Error(w, "Delete failed: "+err.Error(), http.StatusInternalServerError)
		}
	default:
		http.Error(w, "Not allowed", http.StatusMethodNotAllowed)
	}
}

//custom directory walk

func list(path string, info os.FileInfo, node *fsNode, n *uint, rel string) error {
	if (!info.IsDir() && !info.Mode().IsRegular()) || strings.HasPrefix(info.Name(), ".") {
		return errors.New("ERROR: Non-regular file")
	}
	(*n)++
	if (*n) > fileNumberLimit {
		return errors.New("ERROR: Over file limit") //limit number of files walked
	}
	node.Name = info.Name()
	// rel is always slash-separated (URL-friendly)
	node.Path = rel
	node.Size = info.Size()
	node.Modified = info.ModTime()
	if !info.IsDir() {
		return nil
	}
	children, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("ERROR: Failed to list files: %w", err)
	}
	node.Size = 0
	seen := make(map[string]bool)
	for _, i := range children {
		c := &fsNode{}
		p := filepath.Join(path, i.Name())
		childRel := i.Name()
		if rel != "" {
			childRel = rel + "/" + childRel
		}
		childRel = filepath.ToSlash(childRel)
		if err := list(p, i, c, n, childRel); err != nil {
			log.Printf("File listing skipped %s: %s", p, err)
			continue
		}
		if seen[c.Name] {
			log.Printf("Duplicate file skipped: %s", p)
			continue
		}
		seen[c.Name] = true
		node.Size += c.Size
		node.Children = append(node.Children, c)
	}

	return nil
}
