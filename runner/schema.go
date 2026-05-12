package runner

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stringintech/kernel-bindings-tests/docs/schemas"
)

func loadEmbeddedResponseSchemas() (map[string]*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()

	sharedData, err := fs.ReadFile(schemas.FS, "shared.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read shared schema: %w", err)
	}
	sharedDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(sharedData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse shared schema: %w", err)
	}
	if err := compiler.AddResource("shared.json", sharedDoc); err != nil {
		return nil, fmt.Errorf("failed to register shared schema: %w", err)
	}

	methodPaths, err := fs.Glob(schemas.FS, "*.response.json")
	if err != nil {
		return nil, fmt.Errorf("failed to list embedded response schemas: %w", err)
	}

	responseSchemaByMethod := make(map[string]*jsonschema.Schema, len(methodPaths))

	for _, path := range methodPaths {
		data, err := fs.ReadFile(schemas.FS, path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}

		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", path, err)
		}

		if err := compiler.AddResource(path, doc); err != nil {
			return nil, fmt.Errorf("failed to register %s: %w", path, err)
		}

		schema, err := compiler.Compile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to compile %s: %w", path, err)
		}

		responseSchemaByMethod[strings.TrimSuffix(filepath.Base(path), ".response.json")] = schema
	}

	return responseSchemaByMethod, nil
}

func validateJSONAgainstSchema(schema *jsonschema.Schema, data []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return schema.Validate(doc)
}
