package server

// import (
// 	"compress/gzip"
// 	"io"
// 	"net/http"
// 	"net/http/httptest"
// 	"strings"
// 	"testing"
// 	"time"
// )

// func TestHandler(t *testing.T) {
// 	name := "test name"
// 	t.Run("index", func(t *testing.T) {
// 		h := handler{Name: name}
// 		r := httptest.NewRequest("GET", "/", nil)
// 		w := httptest.NewRecorder()
// 		h.ServeHTTP(w, r)
// 		gotBody := w.Body.String()
// 		wantBodyParts := []string{
// 			name,
// 			":root", // css
// 		}
// 		for _, p := range wantBodyParts {
// 			if !strings.Contains(gotBody, p) {
// 				t.Errorf("wanted response to contain %q, got: %q", p, gotBody)
// 			}
// 		}
// 	})
// 	t.Run("not found", func(t *testing.T) {
// 		badURLs := []string{
// 			"/bad.html",
// 		}
// 		for _, url := range badURLs {
// 			t.Run(url, func(t *testing.T) {
// 				var h handler
// 				r := httptest.NewRequest("GET", url, nil)
// 				w := httptest.NewRecorder()
// 				h.ServeHTTP(w, r)
// 				if want, got := 404, w.Code; want != got {
// 					t.Errorf("wanted %v, got %v", want, got)
// 				}
// 			})
// 		}
// 	})
// 	t.Run("other", func(t *testing.T) {
// 		indexURLs := []string{
// 			"/",
// 			"/robots.txt",
// 		}
// 		for _, url := range indexURLs {
// 			t.Run(url, func(t *testing.T) {
// 				var h handler
// 				r := httptest.NewRequest("GET", url, nil)
// 				w := httptest.NewRecorder()
// 				h.ServeHTTP(w, r)
// 				if want, got := 200, w.Code; want != got {
// 					t.Errorf("wanted %v, got %v", want, got)
// 				}
// 			})
// 		}
// 	})
// }

// func TestServerPort(t *testing.T) {
// 	t.Run("in environment", func(t *testing.T) {
// 		want := "1234"
// 		t.Setenv("PORT", want)
// 		got := serverPort()
// 		if want != got {
// 			t.Errorf("wanted %v, got %v", want, got)
// 		}
// 	})
// 	t.Run("not set", func(t *testing.T) {
// 		want := "8000"
// 		got := serverPort()
// 		if want != got {
// 			t.Errorf("wanted %v, got %v", want, got)
// 		}
// 	})
// }

// func TestWithCacheControl(t *testing.T) {
// 	msg := "OK_1549"
// 	h1 := func(w http.ResponseWriter, r *http.Request) {
// 		w.Write([]byte(msg))
// 	}
// 	h2 := withCacheControl(http.HandlerFunc(h1), time.Minute)
// 	r := httptest.NewRequest("", "/", nil)
// 	w := httptest.NewRecorder()
// 	h2.ServeHTTP(w, r)
// 	switch {
// 	case w.Body.String() != msg:
// 		t.Errorf("inner handler not run: wanted body to be %q, got %q", w.Body.String(), msg)
// 	default:
// 		want := "max-age=60"
// 		got := w.Result().Header.Get("Cache-Control")
// 		if want != got {
// 			t.Errorf("missing max-age Cache-Control header: got: %q", got)
// 		}
// 	}
// }

// func TestWithContentEncoding(t *testing.T) {
// 	msg := "OK_gzip"
// 	h1 := func(w http.ResponseWriter, r *http.Request) {
// 		w.Write([]byte(msg))
// 	}
// 	h2 := withContentEncoding(http.HandlerFunc(h1))
// 	w := httptest.NewRecorder()
// 	r := httptest.NewRequest("", "/", nil)
// 	r.Header.Add("Accept-Encoding", "gzip, deflate, br")
// 	h2.ServeHTTP(w, r)
// 	got := w.Result()
// 	switch {
// 	case got.Header.Get("Content-Encoding") != "gzip":
// 		t.Errorf("wanted gzip Content-Encoding, got: %q", got.Header.Get("Content-Encoding"))
// 	default:
// 		r, err := gzip.NewReader(got.Body)
// 		if err != nil {
// 			t.Fatalf("creating gzip reader: %v", err)
// 		}
// 		b, err := io.ReadAll(r)
// 		if err != nil {
// 			t.Fatalf("reading gzip encoded message: %v", err)
// 		}
// 		if want, got := msg, string(b); want != got {
// 			t.Errorf("body not encoded as desired: wanted %q, got %q", want, got)
// 		}
// 	}
// }
