package core

import (
	"bytes"
	"errors"
	"testing"

	"github.com/jprybylski/datum"
)

type failingSchemaWriter struct{}

func (failingSchemaWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestSchema(t *testing.T) {
	var out bytes.Buffer
	if code := Schema(&out); code != 0 {
		t.Fatalf("Schema() = %d, want 0", code)
	}
	if !bytes.Equal(out.Bytes(), datum.ConfigSchema()) {
		t.Fatal("Schema() output differs from the shipped configuration schema")
	}
}

func TestSchemaWriteError(t *testing.T) {
	if code := Schema(failingSchemaWriter{}); code != 1 {
		t.Fatalf("Schema(failing writer) = %d, want 1", code)
	}
}
