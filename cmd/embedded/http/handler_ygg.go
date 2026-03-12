package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"lukechampine.com/blake3"
)

// // // // // // // // // //

const chunkSize = 250 * 1024 // 250 KB

func init() {
	// Register MIME types regardless of OS configuration.
	_ = mime.AddExtensionType(".html", "text/html; charset=utf-8")
	_ = mime.AddExtensionType(".css", "text/css; charset=utf-8")
	_ = mime.AddExtensionType(".js", "application/javascript; charset=utf-8")
	_ = mime.AddExtensionType(".json", "application/json")
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
	_ = mime.AddExtensionType(".png", "image/png")
	_ = mime.AddExtensionType(".ico", "image/x-icon")
	_ = mime.AddExtensionType(".woff2", "font/woff2")
}

// //

type yggFileHandlerObj struct {
	wwwAbs string
	enc    *zstd.Encoder
}

func newYggFileHandler(wwwPath string) http.Handler {
	abs, _ := filepath.Abs(wwwPath)
	enc, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	return &yggFileHandlerObj{wwwAbs: abs, enc: enc}
}

// //

// resolveFile maps a URL path to an absolute file path within wwwAbs.
// Returns the open file and its stat, or an error.
func (h *yggFileHandlerObj) resolveFile(urlPath string) (*os.File, os.FileInfo, error) {
	// Normalize the URL path to prevent traversal.
	clean := path.Clean("/" + strings.TrimPrefix(urlPath, "/"))
	filePath := filepath.Join(h.wwwAbs, filepath.FromSlash(clean))

	// Reject paths that escape the www root.
	fileAbs, err := filepath.Abs(filePath)
	if err != nil || !strings.HasPrefix(fileAbs+string(filepath.Separator), h.wwwAbs+string(filepath.Separator)) {
		return nil, nil, os.ErrPermission
	}

	f, err := os.Open(fileAbs)
	if err != nil {
		return nil, nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}

	// For directories, try index.html.
	if stat.IsDir() {
		_ = f.Close()
		idxPath := filepath.Join(fileAbs, "index.html")
		f, err = os.Open(idxPath)
		if err != nil {
			return nil, nil, os.ErrNotExist
		}
		stat, err = f.Stat()
		if err != nil {
			_ = f.Close()
			return nil, nil, err
		}
	}
	return f, stat, nil
}

func (h *yggFileHandlerObj) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, stat, err := h.resolveFile(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	// Stream-hash file for ETag without loading it fully into memory.
	hasher := blake3.New(32, nil)
	if _, err := io.Copy(hasher, f); err != nil {
		http.Error(w, "hash error", http.StatusInternalServerError)
		return
	}
	etag := `"` + hex.EncodeToString(hasher.Sum(nil)) + `"`

	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Seek back to start for serving.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "seek error", http.StatusInternalServerError)
		return
	}

	ct := mime.TypeByExtension(filepath.Ext(stat.Name()))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)

	acceptsZstd := strings.Contains(r.Header.Get("Accept-Encoding"), "zstd")
	if acceptsZstd {
		w.Header().Set("Content-Encoding", "zstd")
	}

	fileSize := stat.Size()
	flusher, _ := w.(http.Flusher)

	if fileSize > chunkSize {
		h.serveChunked(w, r, f, fileSize, acceptsZstd, flusher)
	} else {
		h.serveSmall(w, f, acceptsZstd)
	}
}

// serveChunked sends the file in ≤250 KB chunks.
// Each chunk's blake3 hash is sent as an HTTP trailer X-Chunk-Hash-{n}.
func (h *yggFileHandlerObj) serveChunked(
	w http.ResponseWriter,
	_ *http.Request,
	f *os.File,
	fileSize int64,
	compress bool,
	flusher http.Flusher,
) {
	chunkCount := int((fileSize + chunkSize - 1) / chunkSize)
	trailerKeys := make([]string, chunkCount)
	for i := range trailerKeys {
		trailerKeys[i] = fmt.Sprintf("X-Chunk-Hash-%d", i)
	}
	// Trailer declaration must happen before the first Write.
	w.Header().Set("Trailer", strings.Join(trailerKeys, ", "))

	buf := make([]byte, chunkSize)
	chunkHashes := make([]string, 0, chunkCount)

	for {
		n, readErr := io.ReadFull(f, buf)
		if n > 0 {
			chunk := buf[:n]
			sum := blake3.Sum256(chunk)
			chunkHashes = append(chunkHashes, hex.EncodeToString(sum[:]))

			var payload []byte
			if compress {
				payload = h.enc.EncodeAll(chunk, nil)
			} else {
				payload = chunk
			}
			_, _ = w.Write(payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}

	// Set trailers — sent by the HTTP server as part of the final chunk.
	for i, hash := range chunkHashes {
		w.Header().Set(trailerKeys[i], hash)
	}
}

// serveSmall compresses and sends a file that fits in a single chunk.
func (h *yggFileHandlerObj) serveSmall(w http.ResponseWriter, f *os.File, compress bool) {
	data, err := io.ReadAll(f)
	if err != nil {
		return
	}
	if compress {
		_, _ = w.Write(h.enc.EncodeAll(data, nil))
	} else {
		_, _ = w.Write(data)
	}
}
