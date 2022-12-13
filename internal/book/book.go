// Package book contains shared book structures
package book

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type (
	// Header contains tne basic identifier of a book
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
		EanIsbn13     string
		UpcIsbn10     string
		ImageBase64   string
	}
	StringBook struct {
		ID            string
		Title         string
		Author        string
		Subject       string
		Description   string
		DeweyDecClass string
		Pages         string
		Publisher     string
		PublishDate   string
		AddedDate     string
		EanIsbn13     string
		UpcIsbn10     string
		ImageBase64   string
	}
	DateLayout string
	Subject    struct {
		Name  string
		Count int
	}
	Books    []Book
	Subjects []Subject
	// Filter is used to match books weth the exact subject (if set) or a whole word match to any of the header parts.
	Filter struct {
		Subject    string
		HeaderPart string
	}
)

const (
	HyphenatedYYYYMMDD DateLayout = "2006-01-02"
	SlashMMDDYYYY      DateLayout = "01/02/2006"
)

// NewID creates a random, url-safe, base64 string.
func NewID() string {
	var src [24]byte
	if _, err := rand.Read(src[:]); err != nil {
		panic("reading random bytes from crypto/rand: " + err.Error())
	}
	return base64.URLEncoding.EncodeToString(src[:])
}

func (books Books) Sort() {
	sort.Slice(books, func(i, j int) bool {
		return books[i].less(books[j].Header)
	})
}

func (h Header) less(other Header) bool {
	if h.Subject != other.Subject {
		return h.Subject < other.Subject
	}
	return h.Title != other.Title
}

func (subjects Subjects) Sort() {
	sort.Slice(subjects, func(i, j int) bool {
		return subjects[i].less(subjects[j])
	})
}

func (s Subject) less(other Subject) bool {
	if s.Name != other.Name {
		return s.Name < other.Name
	}
	return s.Count > other.Count // max first
}

func (f Filter) Matches(b Book) bool {
	if len(f.Subject) != 0 && !strings.EqualFold(f.Subject, b.Subject) {
		return false
	}
	if len(f.HeaderPart) == 0 {
		return true
	}
	headerPart := strings.ToLower(f.HeaderPart)
	for _, part := range []string{b.Title, b.Author, b.Subject} {
		part = strings.ToLower(part)
		if strings.Contains(part, headerPart) {
			return true
		}
	}
	return false
}

func (sb StringBook) Book(dateLayout DateLayout) (*Book, error) {
	b := Book{
		Header: Header{
			ID:      sb.ID,
			Title:   sb.Title,
			Author:  sb.Author,
			Subject: sb.Subject,
		},
		Description:   sb.Description,
		DeweyDecClass: sb.DeweyDecClass,
		Publisher:     sb.Publisher,
		EanIsbn13:     sb.EanIsbn13,
		UpcIsbn10:     sb.UpcIsbn10,
		ImageBase64:   sb.ImageBase64,
	}
	var err error
	if len(sb.Pages) != 0 {
		if b.Pages, err = strconv.Atoi(sb.Pages); err != nil {
			return nil, fmt.Errorf("pages: %w", err)
		}
	}
	if len(sb.PublishDate) != 0 {
		if b.PublishDate, err = time.Parse(string(dateLayout), sb.PublishDate); err != nil {
			return nil, fmt.Errorf("publish date: %w", err)
		}
	}
	if len(sb.AddedDate) != 0 {
		if b.AddedDate, err = time.Parse(string(dateLayout), sb.AddedDate); err != nil {
			return nil, fmt.Errorf("added date: %w", err)
		}
	}
	return &b, nil
}
