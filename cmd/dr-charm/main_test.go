package main

import (
	"bytes"
	"testing"
)

func TestRunVersion(t *testing.T) {
	originalVersion := version
	version = "1.2.3"
	t.Cleanup(func() { version = originalVersion })

	var output bytes.Buffer
	if err := runWithOutput([]string{"-version"}, &output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "dr-charm 1.2.3\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
