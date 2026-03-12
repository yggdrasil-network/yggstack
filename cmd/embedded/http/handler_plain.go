package main

import (
	"net/http"
	"os"
	"path/filepath"
)

// // // // // // // // // //

// noListFSObj wraps http.FileSystem to return 404 for directories without index.html,
// preventing automatic directory listing.
type noListFSObj struct {
	http.FileSystem
}

func (n noListFSObj) Open(name string) (http.File, error) {
	f, err := n.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if stat.IsDir() {
		idx, err := n.FileSystem.Open(filepath.Join(name, "index.html"))
		if err != nil {
			_ = f.Close()
			return nil, os.ErrNotExist
		}
		_ = idx.Close()
	}
	return f, nil
}

// //

func newPlainFileHandler(wwwPath string) http.Handler {
	return http.FileServer(noListFSObj{http.Dir(wwwPath)})
}
