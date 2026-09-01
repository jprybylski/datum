package core

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jprybylski/datum"
	"github.com/jprybylski/datum/internal/registry"
)

func TestConfigurationSchemaFieldsMatchDecoderTypes(t *testing.T) {
	var schema struct {
		Properties  map[string]json.RawMessage `json:"properties"`
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(datum.ConfigSchema(), &schema); err != nil {
		t.Fatal(err)
	}

	assertSameFields(t, "config", yamlFields(reflect.TypeFor[Config]()), keys(schema.Properties))
	assertSameFields(t, "defaults", yamlFields(reflect.TypeFor[Defaults]()), nestedProperties(t, schema.Properties["defaults"]))

	datasets := nestedProperties(t, schema.Properties["datasets"])
	assertSameFields(t, "dataset", yamlFields(reflect.TypeFor[Dataset]()), datasets)

	sourceFields := map[string]bool{}
	for _, definition := range schema.Definitions {
		for field := range definition.Properties {
			sourceFields[field] = true
		}
	}
	assertSameFields(t, "source", yamlFields(reflect.TypeFor[registry.Source]()), keysBool(sourceFields))
}

func nestedProperties(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var property struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Items      struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &property); err != nil {
		t.Fatal(err)
	}
	if property.Properties != nil {
		return keys(property.Properties)
	}
	return keys(property.Items.Properties)
}

func yamlFields(typ reflect.Type) []string {
	fields := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("yaml"), ",")[0]
		if name != "" && name != "-" {
			fields = append(fields, name)
		}
	}
	sort.Strings(fields)
	return fields
}

func keys(values map[string]json.RawMessage) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func keysBool(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func assertSameFields(t *testing.T, name string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s decoder fields = %v, schema fields = %v", name, got, want)
	}
}
