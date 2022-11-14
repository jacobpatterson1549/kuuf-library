package main

import (
	"flag"
	"io"
	"testing"
)

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
			if err := parseFlags(fs, test.args); err != nil {
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
			err := parseFlags(fs, test.args)
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
