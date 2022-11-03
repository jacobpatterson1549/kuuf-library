// Package book contains shared book structures
package book

import (
	"crypto/rand"
	"encoding/base64"
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

// func (b Book) ID() string {
// 	return b.ID
// }

// func (b Book) Title() string {
// 	return b.Title
// }

// func (b Book) Author() string {
// 	return b.Author
// }

// func (b Book) Subject() string {
// 	return b.Subject
// }
