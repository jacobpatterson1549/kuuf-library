// Package book contains shared book structures
package book

import "time"

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
		Description                string
		DeweyDecimalClassification string
		Pages                      int
		Publisher                  string
		PublishDate                time.Time
		AddedDate                  time.Time
		EAN_ISBN13                 string
		UPC_ISBN10                 string
		ImageBase64                string
	}
)
