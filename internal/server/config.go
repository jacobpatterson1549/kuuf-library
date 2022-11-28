package server

import (
	"fmt"
	"io"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/csv"
	"golang.org/x/time/rate"
)

func (cfg Config) queryTimeout() time.Duration {
	return time.Second * time.Duration(cfg.DBTimeoutSec)
}

func (cfg Config) postRateLimiter() *rate.Limiter {
	r := rate.Inf
	if cfg.PostLimitSec != 0 {
		r = 1 / rate.Limit(cfg.PostLimitSec)
	}
	lim := rate.NewLimiter(r, cfg.PostMaxBurst)
	return lim
}

func (cfg Config) setup(db Database, ph PasswordHandler, out io.Writer) error {
	if len(cfg.AdminPassword) != 0 {
		if err := cfg.initAdminPassword(db, ph); err != nil {
			return fmt.Errorf("initializing admin password from server configuration: %w", err)
		}
	}
	if cfg.BackfillCSV {
		if err := cfg.backfillCSV(db); err != nil {
			return fmt.Errorf("backfilling database from internal CSV file: %w", err)
		}
	}
	if cfg.UpdateImages || cfg.DumpCSV {
		if err := cfg.updateImages(db, out); err != nil {
			return fmt.Errorf("updating images / dumping csv;: %w", err)
		}
	}
	return nil
}

func (cfg Config) initAdminPassword(db Database, ph PasswordHandler) error {
	if err := validatePassword(cfg.AdminPassword); err != nil {
		return err
	}
	hashedPassword, err := ph.Hash([]byte(cfg.AdminPassword))
	if err != nil {
		return fmt.Errorf("hashing admin password: %w", err)
	}
	if err := db.UpdateAdminPassword(string(hashedPassword)); err != nil {
		return fmt.Errorf("setting admin password: %w", err)
	}
	return nil
}

func (cfg Config) backfillCSV(db Database) error {
	src, err := embeddedCSVDatabase()
	if err != nil {
		return fmt.Errorf("loading csv database: %w", err)
	}
	books := src.Books
	if _, err := db.CreateBooks(books...); err != nil {
		return fmt.Errorf("creating books: %w", err)
	}
	return nil
}

func (cfg Config) updateImages(db Database, out io.Writer) error {
	d := csv.NewDump(out)
	offset := 0
	for {
		headers, err := db.ReadBookHeaders(book.Filter{}, cfg.MaxRows+1, offset)
		if err != nil {
			return fmt.Errorf("reading books at offset %v: %w", offset, err)
		}
		hasMore := len(headers) > cfg.MaxRows
		if hasMore {
			headers = headers[:cfg.MaxRows]
		}
		for _, h := range headers {
			if err := cfg.updateImage(h, db, *d); err != nil {
				return err
			}
		}
		if !hasMore {
			return nil
		}
		offset += cfg.MaxRows
	}
}

func (cfg Config) updateImage(h book.Header, db Database, d csv.Dump) error {
	b, err := db.ReadBook(h.ID)
	if err != nil {
		return fmt.Errorf("reading book %q: %w", h.ID, err)
	}
	if cfg.UpdateImages && imageNeedsUpdating(b.ImageBase64) {
		imageBase64, err := updateImage(b.ImageBase64, b.ID)
		if err != nil {
			return fmt.Errorf("updating image for book %q: %w", b.ID, err)
		}
		b.ImageBase64 = string(imageBase64)
		if err := db.UpdateBook(*b, true); err != nil {
			return fmt.Errorf("writing updated image to db for book %q: %w", b.ID, err)
		}
	}
	if cfg.DumpCSV {
		d.Write(*b)
	}
	return nil
}

func validatePassword(p string) error {
	if len(p) < 8 {
		return fmt.Errorf("password too short")
	}
	m := make(map[rune]struct{}, len(validPasswordRunes))
	for _, r := range validPasswordRunes {
		m[r] = struct{}{}
	}
	for _, r := range p {
		if _, ok := m[r]; !ok {
			return fmt.Errorf("password contains characters that are not allowed")
		}
	}
	return nil
}
