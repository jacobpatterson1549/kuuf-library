// Package book contains shared book structures
package book

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"
)

type (

	// Book contains tne basic identifier of a book
	Header struct {
		ID      string
		Title   string
		Author  string
		Subject string
	}

	// Book contains common book fields
	Book struct {
		Header
		Description   string
		DeweyDecClass string
		Pages         int
		Publisher     string
		PublishDate   time.Time
		AddedDate     time.Time
		EAN_ISBN13    string
		UPC_ISBN10    string
		ImageBase64   string
	}
)

// NewID creates a random, url-safe, base64 string.
func NewID() string {
	var src [24]byte
	if _, err := rand.Read(src[:]); err != nil {
		panic("reading random bytes from crypto/rand: " + err.Error())
	}
	return base64.URLEncoding.EncodeToString(src[:])
}

type Books []Book

func (books Books) Sort() {
	sort.Slice(books, func(i, j int) bool {
		if books[i].Subject != books[j].Subject {
			return books[i].Subject < books[j].Subject
		}
		return books[i].Title != books[j].Title
	})
}

type Filter []string

func NewFilter(s string) (*Filter, error) {
	if strings.IndexFunc(s, isSpecial) >= 0 {
		return nil, fmt.Errorf("filter can only contain letters, numbers, and spaces")
	}
	f := Filter(strings.Fields(s))
	return &f, nil
}

func (f Filter) Matches(b Book) bool {
	if len(f) == 0 {
		return true
	}
	for _, part := range []string{b.Title, b.Author, b.Subject} {
		for _, w := range strings.Fields(part) {
			for _, v := range f {
				if strings.EqualFold(w, v) {
					return true
				}
			}
		}
	}
	return false
}

func isSpecial(r rune) bool {
	return r != ' ' &&
		!('a' <= r && r <= 'z') &&
		!('A' <= r && r <= 'Z') &&
		!('0' <= r && r <= '9')
}
