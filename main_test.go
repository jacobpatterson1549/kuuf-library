package main

import (
	"flag"
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
