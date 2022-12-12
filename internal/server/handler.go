package server

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func withContextTimeout(h http.Handler, maxDuration time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx, cancelFunc := context.WithTimeout(ctx, maxDuration)
		defer cancelFunc()
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	}
}

// withCacheControl adds a cache-control to GET requests that are not to edit a book
func withCacheControl(h http.Handler, d time.Duration) http.HandlerFunc {
	shouldCache := func(r *http.Request) bool {
		switch {
		case r.Method != http.MethodGet,
			r.URL.Path == "/admin" && r.URL.Query().Has("book-id"): // do not cache book edit read requests
			return false
		}
		return true
	}
	maxAge := "max-age=" + strconv.Itoa(int(d.Seconds()))
	return func(w http.ResponseWriter, r *http.Request) {
		if shouldCache(r) {
			w.Header().Add("Cache-Control", maxAge)
		}
		h.ServeHTTP(w, r)
	}
}

type wrappedResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (wrw wrappedResponseWriter) Write(p []byte) (n int, err error) {
	return wrw.Writer.Write(p)
}

func withContentEncoding(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acceptEncoding := r.Header.Get("Accept-Encoding")
		if !strings.Contains(acceptEncoding, "gzip") {
			h.ServeHTTP(w, r)
			return
		}
		gzw := gzip.NewWriter(w)
		defer gzw.Close()
		wrw := wrappedResponseWriter{
			Writer:         gzw,
			ResponseWriter: w,
		}
		wrw.Header().Set("Content-Encoding", "gzip")
		h.ServeHTTP(wrw, r)
	}
}

type rateLimiter interface {
	Allow() bool
}

func withRateLimiter(h http.HandlerFunc, lim rateLimiter) http.HandlerFunc {
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
