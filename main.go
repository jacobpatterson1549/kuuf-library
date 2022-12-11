package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jacobpatterson1549/kuuf-library/internal/server"
)

func main() {
	ctx := context.Background()
	out := os.Stdout
	programName, programArgs := os.Args[0], os.Args[1:]
	logFlags := log.Ldate | log.Ltime | log.LUTC | log.Lshortfile | log.Lmsgprefix
	log := log.New(out, "", logFlags)
	cfg, err := newServerConfig(out, programName, programArgs...)
	if err != nil {
		log.Fatalf("parsing server config: %v", err)
	}
	go func() {
		s, err := cfg.NewServer(ctx, out)
		if err != nil {
			log.Fatalf("creating server: %v", err)
		}
		if err := s.Run(ctx); err != nil { // BLOCKING
			log.Fatalf("running server: %v", err)
		}
	}()
	done := make(chan os.Signal, 2)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	signal := <-done // BLOCKING
	log.Printf("handled signal: %v", signal)
}

func newServerConfig(out io.Writer, programName string, programArgs ...string) (*server.Config, error) {
	usage := []string{
		programName + " runs a library web server",
	}
	var cfg server.Config
	fs := flag.NewFlagSet(programName, flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() {
		for _, u := range usage {
			fmt.Fprintln(fs.Output(), u)
		}
		fs.PrintDefaults()
	}
	fs.StringVar(&cfg.Port, "port", "8000", "the port to run the server on, required")
	fs.StringVar(&cfg.DatabaseURL, "database-url", "csv://", "the url of the database to use, defaults to the readonly internal library.csv file")
	fs.StringVar(&cfg.AdminPassword, "admin-password", "", "password to set for the administrator, if supplied")
	fs.BoolVar(&cfg.BackfillCSV, "csv-backfill", false, "backfill the database from the internal library.csv file")
	fs.BoolVar(&cfg.DumpCSV, "csv-dump", false, "dump all books from the database to the console as CSV before starting the server")
	fs.BoolVar(&cfg.UpdateImages, "update-images", false, "processes all images in the database to webp")
	fs.IntVar(&cfg.MaxRows, "max-rows", 100, "the maximum number of books to display as rows on the filter page")
	fs.IntVar(&cfg.DBTimeoutSec, "db-timeout-sec", 5, "the number of seconds each database operation can take")
	fs.IntVar(&cfg.PostLimitSec, "post-rate-sec", 5, "the limit on number of seconds that must pas between posts")
	fs.IntVar(&cfg.PostMaxBurst, "post-max-burst", 2, "the maximum number of posts that can take place in a post-rate-sec period")
	if err := parseFlags(fs, programArgs); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// parseFlags parses the flagSet after overlaying environment flags.
// Flags that match the uppercase version of their name, with underscores instead of hyphens are overwritten.
func parseFlags(fs *flag.FlagSet, programArgs []string) error {
	if err := fs.Parse(programArgs); err != nil {
		return fmt.Errorf("parsing program args: %w", err)
	}
	var lastErr error
	fs.VisitAll(func(f *flag.Flag) {
		name := strings.ReplaceAll(strings.ToUpper(f.Name), "-", "_")
		if val, ok := os.LookupEnv(name); ok {
			if err := f.Value.Set(val); err != nil {
				lastErr = err
			}
		}
	})
	if lastErr != nil {
		return fmt.Errorf("setting value from environment variable: %w", lastErr)
	}
	return nil
}
