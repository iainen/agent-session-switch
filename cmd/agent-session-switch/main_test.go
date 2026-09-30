package main

import (
	"bytes"
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want cliArgs
	}{
		{"defaults", nil, cliArgs{limit: 300}},
		{"limit", []string{"-n", "50"}, cliArgs{limit: 50}},
		{"query", []string{"price"}, cliArgs{limit: 300, query: "price"}},
		{"query and limit", []string{"-n", "5", "foo"}, cliArgs{limit: 5, query: "foo"}},
		{"agent args", []string{"--", "--model", "x"}, cliArgs{limit: 300, extra: []string{"--model", "x"}}},
		{"flags before agent args", []string{"-n", "9", "q", "--", "-p", "hi"}, cliArgs{limit: 9, query: "q", extra: []string{"-p", "hi"}}},
		{"empty agent args", []string{"--"}, cliArgs{limit: 300, extra: []string{}}},
		{"version", []string{"-version"}, cliArgs{limit: 300, showVersion: true}},
		{"double dash version", []string{"--version"}, cliArgs{limit: 300, showVersion: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, args := range [][]string{
		{"-n", "0"},
		{"-n", "-3"},
		{"-n", "abc"},
		{"-nope"},
		{"one", "two"},
	} {
		if _, err := parseArgs(args, &bytes.Buffer{}); err == nil {
			t.Errorf("parseArgs(%v) succeeded, want an error", args)
		}
	}
}

func TestParseArgsHelp(t *testing.T) {
	var out bytes.Buffer
	_, err := parseArgs([]string{"-h"}, &out)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want flag.ErrHelp", err)
	}
	for _, want := range []string{"Usage:", "-n", "-version"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help output is missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "agent-session-switch ") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-n", "0"}, &out, &errOut); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunHelpExitsZero(t *testing.T) {
	if code := run([]string{"-h"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}
