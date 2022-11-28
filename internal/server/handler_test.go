package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWithCacheControl(t *testing.T) {
	msg := "OK_1549"
	h1 := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(msg))
	}
	tests := []struct {
		name             string
		wantCacheControl bool
		r                *http.Request
	}{
		{"subjects get", true, httptest.NewRequest("GET", "/", nil)},
		{"book get", true, httptest.NewRequest("GET", "/book?id=existing", nil)},
		{"add book get", true, httptest.NewRequest("GET", "/admin", nil)},
		{"edit book get", false, httptest.NewRequest("GET", "/admin?book-id=existing", nil)},
		{"add book post", false, httptest.NewRequest("POST", "/admin", nil)},
		{"list", true, httptest.NewRequest("GET", "/list", nil)},
		{"list  search", true, httptest.NewRequest("GET", "/list?q=search", nil)},
		{"book update", false, httptest.NewRequest("POST", "/book?id=existing", nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h2 := withCacheControl(http.HandlerFunc(h1), time.Minute)
			w := httptest.NewRecorder()
			h2.ServeHTTP(w, test.r)
			switch {
			case w.Body.String() != msg:
				t.Errorf("inner handler not run: wanted body to be %q, got %q", w.Body.String(), msg)
			default:
				want := "max-age=60"
				got := w.Result().Header.Get("Cache-Control")
				gotCacheControl := want == got
				if test.wantCacheControl != gotCacheControl {
					t.Errorf("wanted cache control: %v, got: %q", test.wantCacheControl, got)
				}
			}
		})
	}
}

func TestWithContentEncoding(t *testing.T) {
	msg := "OK_gzip"
	h1 := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(msg))
	}
	h2 := withContentEncoding(http.HandlerFunc(h1))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("", "/", nil)
	r.Header.Add("Accept-Encoding", "gzip, deflate, br")
	h2.ServeHTTP(w, r)
	got := w.Result()
	switch {
	case got.Header.Get("Content-Encoding") != "gzip":
		t.Errorf("wanted gzip Content-Encoding, got: %q", got.Header.Get("Content-Encoding"))
	default:
		r, err := gzip.NewReader(got.Body)
		if err != nil {
			t.Fatalf("creating gzip reader: %v", err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("reading gzip encoded message: %v", err)
		}
		if want, got := msg, string(b); want != got {
			t.Errorf("body not encoded as desired: wanted %q, got %q", want, got)
		}
	}
}

func TestWithRateLimiter(t *testing.T) {
	tests := []struct {
		name        string
		wantCode    int
		lim         rateLimiter
		numRequests int
	}{
		{"zero burst", 429, &countRateLimiter{}, 1},
		{"first allowed", 200, &countRateLimiter{max: 1}, 1},
		{"third allowed", 200, &countRateLimiter{max: 4}, 3},
		{"fifth not allowed", 429, &countRateLimiter{max: 4}, 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h1 := func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			}
			var w *httptest.ResponseRecorder
			h2 := withRateLimiter(h1, test.lim)
			r := httptest.NewRequest("POST", "/admin", nil)
			for i := 0; i < test.numRequests; i++ {
				w = httptest.NewRecorder()
				h2.ServeHTTP(w, r)
			}
			if want, got := test.wantCode, w.Code; want != got {
				t.Errorf("status codes not equal: wanted %v, got %v", want, got)
			}
		})
	}
}
