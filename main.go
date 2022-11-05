package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jacobpatterson1549/kuuf-library/internal/server"
)

func main() {
	programName, programArgs := os.Args[0], os.Args[1:]
	usage := []string{
		"runs a library web server",
	}
	var cfg server.Config
	fs := flag.NewFlagSet(programName, flag.ExitOnError)
	fs.Usage = func() {
		for _, u := range usage {
			fmt.Println(fs.Output(), u)
		}
		fs.PrintDefaults()
	}
	fs.StringVar(&cfg.Port, "port", "8000", "the port to run the server on, required")
	fs.StringVar(&cfg.DatabaseURL, "database-url", "csv://", "the url of the database to use, defaults to the readonly internal library.csv file")
	fs.StringVar(&cfg.AdminPassword, "admin-password", "", "password to set for the administrator, if supplied")
	fs.BoolVar(&cfg.BackfillCSV, "csv-backfill", false, "backfill the database from the internal library.csv file")
	fs.BoolVar(&cfg.DumpCSV, "csv-dump", false, "dump all books from the database to the console as CSV before starting the server")
	fs.IntVar(&cfg.MaxRows, "max-rows", 100, "the maximum number of books to display as rows on the filter page")
	fs.IntVar(&cfg.DBTimeoutSec, "db-timeout-sec", 5, "the number of seconds each database operation can take")
	if err := parseFlags(fs, programArgs); err != nil {
		log.Fatalf("parsing server args: %v", err)
	}
	s, err := cfg.NewServer()
	if err != nil {
		log.Fatalf("creating server: %v", err)
	}
	log.Fatal(s.Run())
}

// parseFlags parses the flagSet after overlaying environment flags.
// Flags that match the uppercase version of their name are overwritten.
func parseFlags(fs *flag.FlagSet, programArgs []string) error {
	if err := fs.Parse(programArgs); err != nil {
		return fmt.Errorf("parsing program args: %w", err)
	}
	var lastErr error
	fs.VisitAll(func(f *flag.Flag) {
		name := strings.ToUpper(f.Name)
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
