package server

import (
	"context"
	"fmt"
	"io"
	"strings"
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

func (cfg Config) databaseScheme() string {
	beforeColon, _, _ := strings.Cut(cfg.DatabaseURL, ":")
	return beforeColon
}

func (cfg Config) setup(ctx context.Context, db database, ph passwordHandler, out io.Writer) error {
	if len(cfg.AdminPassword) != 0 {
		if err := cfg.initAdminPassword(ctx, db, ph); err != nil {
			return fmt.Errorf("initializing admin password from server configuration: %w", err)
		}
	}
	if cfg.BackfillCSV {
		if err := cfg.backfillCSV(ctx, db); err != nil {
			return fmt.Errorf("backfilling database from internal CSV file: %w", err)
		}
	}
	if cfg.UpdateImages || cfg.DumpCSV {
		if err := cfg.updateImages(ctx, db, out); err != nil {
			return fmt.Errorf("updating images / dumping csv;: %w", err)
		}
	}
	return nil
}

func (cfg Config) initAdminPassword(ctx context.Context, db database, ph passwordHandler) error {
	if err := validatePassword(cfg.AdminPassword); err != nil {
		return err
	}
	hashedPassword, err := ph.Hash([]byte(cfg.AdminPassword))
	if err != nil {
		return fmt.Errorf("hashing admin password: %w", err)
	}
	if err := db.UpdateAdminPassword(ctx, string(hashedPassword)); err != nil {
		return fmt.Errorf("setting admin password: %w", err)
	}
	return nil
}

func (cfg Config) backfillCSV(ctx context.Context, db database) error {
	csvD, err := embeddedCSVDatabase()
	if err != nil {
		return fmt.Errorf("loading csv database: %w", err)
	}
	iter := newBookIterator(csvD, cfg.MaxRows)
	books, err := iter.AllBooks(ctx)
	if err != nil {
		return fmt.Errorf("reading all books to backfill: %v", err)
	}
	if _, err := db.CreateBooks(ctx, books...); err != nil {
		return fmt.Errorf("creating books: %w", err)
	}
	return nil
}

func (cfg Config) updateImages(ctx context.Context, db database, out io.Writer) error {
	d := csv.NewDump(out)
	iter := newBookIterator(db, cfg.MaxRows)
	for iter.HasNext(ctx) {
		b, err := iter.Next(ctx)
		if err != nil {
			return err
		}
		if err := cfg.updateImage(ctx, *b, db, *d); err != nil {
			return err
		}
		if cfg.DumpCSV {
			d.Write(*b)
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	return nil
}

func (cfg Config) updateImage(ctx context.Context, b book.Book, db database, d csv.Dump) error {
	if !cfg.UpdateImages || !imageNeedsUpdating(b.ImageBase64) {
		return nil
	}
	imageBase64, err := updateImage(b.ImageBase64, b.ID)
	if err != nil {
		return fmt.Errorf("updating image for book %q: %w", b.ID, err)
	}
	b.ImageBase64 = string(imageBase64)
	if err := db.UpdateBook(ctx, b, true); err != nil {
		return fmt.Errorf("writing updated image to db for book %q: %w", b.ID, err)
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
