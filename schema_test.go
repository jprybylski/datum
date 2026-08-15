package datum

import (
	"bytes"
	"os"
	"testing"
)

func TestGeneratedSchemaMatchesSource(t *testing.T) {
	want, err := os.ReadFile("data-schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := ConfigSchema(); !bytes.Equal(got, want) {
		t.Fatal("embedded schema is stale; run `go generate .`")
	}
}

func TestSourceTypeSpecsComeFromSchema(t *testing.T) {
	specs, err := SourceTypeSpecs()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"command", "file", "git", "http"}
	if len(specs) != len(want) {
		t.Fatalf("SourceTypeSpecs() returned %d types, want %d", len(specs), len(want))
	}
	for i, name := range want {
		if specs[i].Type != name {
			t.Errorf("SourceTypeSpecs()[%d].Type = %q, want %q", i, specs[i].Type, name)
		}
		if len(specs[i].Fields) == 0 || specs[i].Fields[0].Name != "type" {
			t.Errorf("SourceTypeSpecs()[%d] does not begin with its required type field", i)
		}
	}
}
