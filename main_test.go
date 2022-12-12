package main

import (
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/jacobpatterson1549/kuuf-library/internal/server"
)

func TestServerConfig(t *testing.T) {
	tests := []struct {
		name        string
		programName string
		programArgs []string
		programEnv  [][]string
		wantOk      bool
		wantLogPart string
		want        *server.Config
	}{
		{
			name:        "unknown arg",
			programArgs: []string{"-delete-db=true"},
		},
		{
			name:        "bad max rows",
			programArgs: []string{"-max-rows=many"},
		},
		{
			name:        "display help",
			programName: "libraryXYZ",
			programArgs: []string{"-h"},
			wantLogPart: "libraryXYZ",
		},
		{
			name:   "default args",
			wantOk: true,
			want: &server.Config{
				Port:         "8000",
				DatabaseURL:  "csv://",
				MaxRows:      100,
				DBTimeoutSec: 5,
				PostLimitSec: 5,
				PostMaxBurst: 2,
			},
		},
		{
			name:   "from program args",
			wantOk: true,
			programArgs: []string{
				"-port=8001",
				"-database-url=postgres://u:p@localhost/kuuf_library_db1",
				"-admin-password=new-password1",
				"-csv-backfill=true",
				"-csv-dump=true",
				"-update-images=true",
				"-max-rows=30",
				"-db-timeout-sec=4",
				"-post-rate-sec=6",
				"-post-max-burst=3",
			},
			want: &server.Config{
				Port:          "8001",
				DatabaseURL:   "postgres://u:p@localhost/kuuf_library_db1",
				AdminPassword: "new-password1",
				BackfillCSV:   true,
				DumpCSV:       true,
				UpdateImages:  true,
				MaxRows:       30,
				DBTimeoutSec:  4,
				PostLimitSec:  6,
				PostMaxBurst:  3,
			},
		},
		{
			name:   "from program env",
			wantOk: true,
			programEnv: [][]string{
				{"PORT", "8002"},
				{"DATABASE_URL", "postgres://u:p@localhost/kuuf_library_db2"},
				{"ADMIN_PASSWORD", "new-password2"},
				{"CSV_BACKFILL", "true"},
				{"CSV_DUMP", "true"},
				{"UPDATE_IMAGES", "true"},
				{"MAX_ROWS", "55"},
				{"DB_TIMEOUT_SEC", "3"},
				{"POST_RATE_SEC", "7"},
				{"POST_MAX_BURST", "4"},
			},
			want: &server.Config{
				Port:          "8002",
				DatabaseURL:   "postgres://u:p@localhost/kuuf_library_db2",
				AdminPassword: "new-password2",
				BackfillCSV:   true,
				DumpCSV:       true,
				UpdateImages:  true,
				MaxRows:       55,
				DBTimeoutSec:  3,
				PostLimitSec:  7,
				PostMaxBurst:  4,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, kv := range test.programEnv {
				t.Setenv(kv[0], kv[1])
			}
			var sb strings.Builder
			got, err := newServerConfig(&sb, test.programName, test.programArgs...)
			switch {
			case !strings.Contains(sb.String(), test.wantLogPart):
				t.Errorf("wanted log to contain %q, got: %v", test.wantLogPart, sb.String())
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case *test.want != *got:
				t.Errorf("configs not equal: \n wanted: %+v \n got:    %+v", test.want, got)
			}
		})
	}
}

func TestParseFlagSet(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  [][]string
		want string
	}{
		{"nothing defined", nil, nil, "defaultV"},
		{"only arg defined", []string{"-p=1"}, nil, "1"},
		{"only env defined", nil, [][]string{{"P", "2"}}, "2"},
		{"both defined", []string{"-p=1"}, [][]string{{"P", "2"}}, "2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, kv := range test.env {
				t.Setenv(kv[0], kv[1])
			}
			var p string
			fs := flag.NewFlagSet("", 0)
			fs.StringVar(&p, "p", "defaultV", "")
			if err := ParseFlags(fs, test.args); err != nil {
				t.Fatalf("unwanted error: %v", err)
			}
			if want, got := test.want, p; want != got {
				t.Errorf("wanted %q, got %q", want, got)
			}
		})
	}
}

func TestParseFlagSetInteger(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		env    [][]string
		wantOk bool
		want   int
	}{
		{"valid integer default", nil, nil, true, 1},
		{"valid integer arg", []string{"-p=2"}, nil, true, 2},
		{"valid integer env", nil, [][]string{{"P", "3"}}, true, 3},
		{"invalid integer arg", []string{"-p=four"}, nil, false, 0},
		{"invalid integer env", nil, [][]string{{"P", "five"}}, false, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, kv := range test.env {
				t.Setenv(kv[0], kv[1])
			}
			fs := flag.NewFlagSet("", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			var p int
			fs.IntVar(&p, "p", 1, "")
			err := ParseFlags(fs, test.args)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case p != test.want:
				t.Errorf("values not equal: wanted: %v, got %v", test.want, p)
			}
		})
	}
}
