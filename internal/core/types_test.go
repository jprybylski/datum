package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jprybylski/datum"
)

var testTypeSpecs = []datum.SourceTypeSpec{
	{
		Type: "alpha", Description: "Alpha source",
		Fields: []datum.SourceFieldSpec{
			{Name: "type", Required: true, Description: `Must be "alpha".`},
			{Name: "path", Description: "Optional source path."},
		},
	},
	{
		Type: "beta", Description: "Beta source",
		Fields: []datum.SourceFieldSpec{{Name: "type", Required: true, Description: `Must be "beta".`}},
	},
}

func testSpecs() ([]datum.SourceTypeSpec, error) { return testTypeSpecs, nil }
func testNames() []string                        { return []string{"alpha", "beta"} }
func testRegistered(name string) bool            { return name == "alpha" || name == "beta" }

func TestTypesList(t *testing.T) {
	var out bytes.Buffer
	if code := types(nil, &out, testSpecs, testNames, testRegistered); code != 0 {
		t.Fatalf("types(list) = %d, want 0", code)
	}
	for _, want := range []string{"Available dataset source types:", "alpha", "Alpha source", "datum types TYPE"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list output missing %q:\n%s", want, out.String())
		}
	}
}

func TestTypesDetails(t *testing.T) {
	var out bytes.Buffer
	if code := types([]string{"alpha", "beta"}, &out, testSpecs, testNames, testRegistered); code != 0 {
		t.Fatalf("types(details) = %d, want 0", code)
	}
	for _, want := range []string{"alpha\n", "beta\n", "(required)", "(optional)", "Optional source path."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("detail output missing %q:\n%s", want, out.String())
		}
	}
}

func TestTypesJSON(t *testing.T) {
	JSONOutput = true
	defer func() { JSONOutput = false }()

	var out bytes.Buffer
	if code := types([]string{"alpha"}, &out, testSpecs, testNames, testRegistered); code != 0 {
		t.Fatalf("types(JSON) = %d, want 0", code)
	}
	var report struct {
		Types []datum.SourceTypeSpec `json:"types"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Types) != 1 || report.Types[0].Type != "alpha" {
		t.Fatalf("JSON types = %+v, want alpha", report.Types)
	}
}

func TestTypesErrors(t *testing.T) {
	t.Run("schema load", func(t *testing.T) {
		var out bytes.Buffer
		code := types(nil, &out, func() ([]datum.SourceTypeSpec, error) {
			return nil, errors.New("broken schema")
		}, testNames, testRegistered)
		if code != 1 || !strings.Contains(out.String(), "broken schema") {
			t.Fatalf("code = %d, output = %q", code, out.String())
		}
	})

	t.Run("registered type absent from schema", func(t *testing.T) {
		var out bytes.Buffer
		code := types(nil, &out, testSpecs, func() []string { return []string{"missing"} }, testRegistered)
		if code != 1 || !strings.Contains(out.String(), "missing from the configuration schema") {
			t.Fatalf("code = %d, output = %q", code, out.String())
		}
	})

	t.Run("unknown requested type", func(t *testing.T) {
		var out bytes.Buffer
		code := types([]string{"missing"}, &out, testSpecs, testNames, testRegistered)
		if code != 2 || !strings.Contains(out.String(), `unknown dataset source type "missing"`) {
			t.Fatalf("code = %d, output = %q", code, out.String())
		}
	})

	t.Run("documented but unavailable type", func(t *testing.T) {
		var out bytes.Buffer
		code := types([]string{"alpha"}, &out, testSpecs, testNames, func(string) bool { return false })
		if code != 2 || !strings.Contains(out.String(), `unknown dataset source type "alpha"`) {
			t.Fatalf("code = %d, output = %q", code, out.String())
		}
	})

	t.Run("JSON error document", func(t *testing.T) {
		JSONOutput = true
		defer func() { JSONOutput = false }()
		var out bytes.Buffer
		code := types([]string{"missing"}, &out, testSpecs, testNames, testRegistered)
		if code != 2 || !json.Valid(out.Bytes()) || !strings.Contains(out.String(), `"error"`) {
			t.Fatalf("code = %d, output = %q", code, out.String())
		}
	})

	t.Run("JSON output write", func(t *testing.T) {
		JSONOutput = true
		defer func() { JSONOutput = false }()
		if code := types(nil, failingSchemaWriter{}, testSpecs, testNames, testRegistered); code != 1 {
			t.Fatalf("types(failing writer) = %d, want 1", code)
		}
	})
}
