package server

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// withCacheControl adds a cache-control to GET requests that are not to edit a book
func withCacheControl(h http.Handler, d time.Duration) http.HandlerFunc {
	maxAge := "max-age=" + strconv.Itoa(int(d.Seconds()))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && (r.URL.Path != "/admin" || len(r.URL.Query()["book-id"]) == 0) {
			w.Header().Add("Cache-Control", maxAge)
		}
		h.ServeHTTP(w, r)
	}
}

func withContentEncoding(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"):
			gzw := gzip.NewWriter(w)
			defer gzw.Close()
			wrw := wrappedResponseWriter{
				Writer:         gzw,
				ResponseWriter: w,
			}
			wrw.Header().Set("Content-Encoding", "gzip")
			h.ServeHTTP(wrw, r)
		default:
			h.ServeHTTP(w, r)
		}
	}
}

type wrappedResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (wrw wrappedResponseWriter) Write(p []byte) (n int, err error) {
	return wrw.Writer.Write(p)
}

func withRateLimiter(h http.HandlerFunc, lim *rate.Limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !lim.Allow() {
			err := fmt.Errorf("too many POSTS to server")
			httpError(w, http.StatusTooManyRequests, err)
			return
		}
		h.ServeHTTP(w, r)
	}
}

// mux is http Handler that maps methods to paths to handlers.
type mux map[string]map[string]http.HandlerFunc

// ServeHTTP serves to the path for the method of the request on the handler if such a Handler exists.
func (m mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	methodHandlers, ok := m[r.Method]
	if !ok {
		httpError(w, http.StatusMethodNotAllowed, nil)
		return
	}
	h, ok := methodHandlers[r.URL.Path]
	if !ok {
		httpError(w, http.StatusNotFound, nil)
		return
	}
	h.ServeHTTP(w, r)
}
