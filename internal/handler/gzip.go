package handler

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

type gzipBody struct {
	io.Reader
	io.Closer
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	statusCode  int
	wroteHeader bool
	pending     bool
	compress    bool
	closed      bool
}

func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil &&
			r.ContentLength != 0 &&
			strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "gzip") {

			gzipReader, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "invalid gzip body", http.StatusBadRequest)
				return
			}

			defer gzipReader.Close()

			r.Body = &gzipBody{
				Reader: gzipReader,
				Closer: r.Body,
			}

			r.ContentLength = -1
			r.Header.Del("Content-Encoding")
			r.Header.Del("Content-Length")
		}

		if !strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gzipWriter := gzip.NewWriter(w)

		gw := &gzipResponseWriter{
			ResponseWriter: w,
			gz:             gzipWriter,
		}

		w.Header().Add("Vary", "Accept-Encoding")

		defer func() {
			_ = gw.Close()
		}()

		next.ServeHTTP(gw, r)
	})
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.wroteHeader || w.closed || w.pending {
		return
	}

	w.statusCode = code
	w.pending = true
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if w.closed {
		return 0, http.ErrBodyNotAllowed
	}

	if !w.wroteHeader {
		if w.Header().Get("Content-Type") == "" && len(b) > 0 {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}

		w.flushHeader()
	}

	if w.compress {
		return w.gz.Write(b)
	}

	return w.ResponseWriter.Write(b)
}

func (w *gzipResponseWriter) Flush() {
	if w.closed {
		return
	}

	w.flushHeader()

	if w.compress {
		_ = w.gz.Flush()
	}

	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *gzipResponseWriter) Close() error {
	if w.closed {
		return nil
	}

	w.closed = true

	if !w.wroteHeader {
		if w.statusCode == 0 {
			w.statusCode = http.StatusOK
		}

		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(w.statusCode)
	}

	if w.compress {
		return w.gz.Close()
	}

	return nil
}

func (w *gzipResponseWriter) flushHeader() {
	if w.wroteHeader {
		return
	}

	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}

	if isCompressibleContentType(w.Header().Get("Content-Type")) {
		w.compress = true
		w.Header().Set("Content-Encoding", "gzip")

		w.Header().Del("Content-Length")
	}

	w.wroteHeader = true
	w.pending = false

	w.ResponseWriter.WriteHeader(w.statusCode)
}

func isCompressibleContentType(contentType string) bool {
	contentType = strings.ToLower(contentType)

	return strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/html")
}