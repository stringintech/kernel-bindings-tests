package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	docOrderKeyword  = "x-doc-order"
	docOrderVocabURL = "urn:kernel-bindings-tests:specgen:doc-order"
)

var docOrderVocabulary = mustDocOrderVocabulary()

type docOrderExt struct {
	Order []string
}

func (e *docOrderExt) Validate(*jsonschema.ValidatorContext, any) {}

func compileSchema(path string) (*jsonschema.Schema, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	c := jsonschema.NewCompiler()
	c.AssertVocabs()
	// Preserve x-doc-order so generated parameter docs follow the schema's
	// reader-facing order instead of the map iteration order.
	c.RegisterVocabulary(docOrderVocabulary)
	return c.Compile(abs)
}

// docOrder reads the custom x-doc-order extension attached during compilation.
// The schema library keeps object properties in a map, so this preserves the
// reader-facing order declared in the source schema.
func docOrder(schema *jsonschema.Schema) []string {
	schema = resolveSchema(schema)
	if schema == nil {
		return nil
	}
	for _, ext := range schema.Extensions {
		docExt, ok := ext.(*docOrderExt)
		if ok {
			return docExt.Order
		}
	}
	return nil
}

// resolveSchema unwraps compiled $ref wrappers to the concrete target schema.
// The jsonschema library resolves references during compilation; this helper
// just follows Schema.Ref pointers so callers can inspect the target fields.
func resolveSchema(schema *jsonschema.Schema) *jsonschema.Schema {
	for schema != nil {
		if schema.Ref == nil {
			return schema
		}
		schema = schema.Ref
	}
	return nil
}

// mustDocOrderVocabulary registers a tiny custom vocabulary just for x-doc-order.
// The compiled extension is later read back by docOrder during markdown rendering.
func mustDocOrderVocabulary() *jsonschema.Vocabulary {
	meta, err := jsonschema.UnmarshalJSON(strings.NewReader(`{
		"properties": {
			"x-doc-order": {
				"type": "array",
				"items": { "type": "string" },
				"uniqueItems": true
			}
		}
	}`))
	if err != nil {
		panic(err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource(docOrderVocabURL, meta); err != nil {
		panic(err)
	}
	schema, err := c.Compile(docOrderVocabURL)
	if err != nil {
		panic(err)
	}

	return &jsonschema.Vocabulary{
		URL:    docOrderVocabURL,
		Schema: schema,
		// Compile stores x-doc-order on the compiled schema for later doc rendering.
		Compile: compileDocOrder,
	}
}

// compileDocOrder validates the raw extension payload and stores it on the
// compiled schema as a strongly typed helper value.
func compileDocOrder(_ *jsonschema.CompilerContext, obj map[string]any) (jsonschema.SchemaExt, error) {
	raw, ok := obj[docOrderKeyword]
	if !ok {
		return nil, nil
	}

	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", docOrderKeyword)
	}

	order := make([]string, 0, len(values))
	for _, value := range values {
		name, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s entries must be strings", docOrderKeyword)
		}
		order = append(order, name)
	}
	return &docOrderExt{Order: order}, nil
}
