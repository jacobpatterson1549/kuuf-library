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
	out := os.Stdout
	logFlags := log.Ldate | log.Ltime | log.LUTC | log.Lshortfile | log.Lmsgprefix
	log := log.New(out, "", logFlags)
	cfg, err := serverConfig(programName, programArgs)
	if err != nil {
		log.Fatalf("parsing server config:  %v", err)
	}
	s, err := cfg.NewServer(out)
	if err != nil {
		log.Fatalf("creating server: %v", err)
	}
	if err := s.Run(); err != nil {
		log.Fatalf("running server: %v", err)
	}
}

func serverConfig(programName string, programArgs []string) (*server.Config, error) {
	usage := []string{
		"runs a library web server",
	}
	var cfg server.Config
	fs := flag.NewFlagSet(programName, flag.ExitOnError)
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
