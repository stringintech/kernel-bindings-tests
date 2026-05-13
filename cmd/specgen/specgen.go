package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type methodDoc struct {
	Name    string
	Summary string
	Params  []paramDoc
	Result  string
	Error   string
}

type paramDoc struct {
	Name        string
	Type        string
	Required    bool
	Description string
	Children    []paramDoc
	EnumValues  []string
}

const (
	kindRefObject            = "ref-object"
	kindGenericErrorResponse = "generic-error-response"
	kindRefResponse          = "ref-response"
	kindNullResponse         = "null-response"
	kindBooleanResponse      = "boolean-response"
	kindIntegerResponse      = "integer-response"
	kindHexStringResponse    = "hex-string-response"
	kindHex64StringResponse  = "hex64-string-response"
)

// GenerateMethodReference builds method reference markdown from method schemas in schemaDir.
func GenerateMethodReference(schemaDir, outPath string) error {
	refs, err := methodSchemaPaths(schemaDir)
	if err != nil {
		return err
	}

	methods, err := loadMethodDocs(refs)
	if err != nil {
		return err
	}

	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Name < methods[j].Name
	})

	return os.WriteFile(outPath, []byte(renderMethods(methods)), 0o644)
}

func methodSchemaPaths(schemaDir string) ([]string, error) {
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		if name == "suite-schema.json" || name == "shared.json" || strings.HasSuffix(name, ".response.json") {
			continue
		}

		paths = append(paths, filepath.Join(schemaDir, name))
	}

	return paths, nil
}

func loadMethodDocs(refs []string) ([]methodDoc, error) {
	methods := make([]methodDoc, 0, len(refs))
	for _, ref := range refs {
		doc, err := loadMethodDoc(ref)
		if err != nil {
			return nil, err
		}
		methods = append(methods, doc)
	}
	return methods, nil
}

// Method extraction: parse one method schema into documentation fields.
func loadMethodDoc(schemaPath string) (methodDoc, error) {
	schema, err := compileSchema(schemaPath)
	if err != nil {
		return methodDoc{}, err
	}

	methodName, err := stringConst(schemaAt(schema, "request", "method"))
	if err != nil {
		return methodDoc{}, fmt.Errorf("%s: missing request.method.const: %w", schemaPath, err)
	}

	params, err := loadParams(schema, schemaPath)
	if err != nil {
		return methodDoc{}, err
	}

	responseSchema := schemaAt(schema, "expected_response")
	if responseSchema == nil {
		return methodDoc{}, fmt.Errorf("%s: missing expected_response schema", schemaPath)
	}

	result, errText := loadResponseDocs(responseSchema)

	return methodDoc{
		Name:    methodName,
		Summary: markdownSentence(schema.Description),
		Params:  params,
		Result:  result,
		Error:   errText,
	}, nil
}

func loadParams(schema *jsonschema.Schema, schemaPath string) ([]paramDoc, error) {
	paramsSchema := schemaAt(schema, "request", "params")
	if paramsSchema == nil {
		return nil, fmt.Errorf("%s: missing request.params schema", schemaPath)
	}
	return renderProperties(paramsSchema)
}

// loadResponseDocs turns the expected response schema into one markdown line for
// the success case and one for the error case. Schemas often model this as a
// oneOf between a normal response object and an error envelope.
func loadResponseDocs(response *jsonschema.Schema) (string, string) {
	result := fallbackResultText(response)
	errText := "`null` (cannot return error)"

	if branches := oneOfSchemas(response); len(branches) > 0 {
		for _, branch := range branches {
			if isErrorResponseSchema(branch) {
				if desc := schemaDescription(branch); desc != "" {
					errText = desc
				} else {
					errText = fallbackErrorText(branch)
				}
				continue
			}
			if desc := schemaDescription(branch); desc != "" {
				result = desc
			} else if txt := fallbackResultText(branch); txt != "" {
				result = txt
			}
		}
		return result, errText
	}

	if isErrorResponseSchema(response) {
		result = "`null`"
		if desc := schemaDescription(response); desc != "" {
			errText = desc
		} else {
			errText = fallbackErrorText(response)
		}
		return result, errText
	}

	if desc := schemaDescription(response); desc != "" {
		result = desc
	}

	return result, errText
}

// Markdown rendering helpers.
func renderMethods(methods []methodDoc) string {
	parts := make([]string, 0, len(methods)+1)
	parts = append(parts,
		"## Method Reference\n\nMethods are sorted alphabetically. Each method documents its parameters, return values, and possible errors.",
	)

	methodParts := make([]string, 0, len(methods))
	for _, method := range methods {
		methodParts = append(methodParts, renderMethod(method))
	}
	if len(methodParts) > 0 {
		parts = append(parts, strings.Join(methodParts, "\n\n---\n\n"))
	}

	return strings.Join(parts, "\n\n") + "\n"
}

func renderMethod(method methodDoc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#### `%s`\n\n", method.Name)
	if method.Summary != "" {
		b.WriteString(method.Summary)
		b.WriteString("\n\n")
	}
	writeParams(&b, method.Params)
	fmt.Fprintf(&b, "**%s:** %s\n\n", "Result", method.Result)
	fmt.Fprintf(&b, "**%s:** %s", "Error", method.Error)
	return b.String()
}

// writeParams emits a nested markdown bullet list. Child params appear under
// object params, and enum values become a second nesting level.
func writeParams(b *strings.Builder, params []paramDoc) {
	b.WriteString("**Parameters:**\n")
	if len(params) == 0 {
		b.WriteString("- None\n\n")
		return
	}
	for _, p := range params {
		writeParam(b, p, 0)
	}
	b.WriteString("\n")
}

func writeParam(b *strings.Builder, p paramDoc, depth int) {
	indent := strings.Repeat("  ", depth)
	req := "optional"
	if p.Required {
		req = "required"
	}
	line := fmt.Sprintf("%s- `%s` (%s, %s)", indent, p.Name, p.Type, req)
	if p.Description != "" {
		line += ": " + p.Description
	}
	if len(p.EnumValues) > 0 {
		line += enumLeadIn(p.Description)
	}
	b.WriteString(line)
	b.WriteString("\n")
	for _, child := range p.Children {
		writeParam(b, child, depth+1)
	}
	if len(p.EnumValues) > 0 {
		for _, v := range p.EnumValues {
			fmt.Fprintf(b, "%s  - `%s`\n", strings.Repeat("  ", depth+1), v)
		}
	}
}

// Parameter rendering and schema-to-doc formatting.
func renderProperties(schema *jsonschema.Schema) ([]paramDoc, error) {
	if schema == nil || len(schema.Properties) == 0 {
		return nil, nil
	}

	required := requiredSet(schema)
	names, err := orderedPropertyNames(schema)
	if err != nil {
		return nil, err
	}
	out := make([]paramDoc, 0, len(names))
	for _, name := range names {
		param, err := renderParam(name, schema.Properties[name], required[name])
		if err != nil {
			return nil, err
		}
		out = append(out, param)
	}

	return out, nil
}

func requiredSet(schema *jsonschema.Schema) map[string]bool {
	required := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = true
	}
	return required
}

// orderedPropertyNames enforces x-doc-order for multi-field objects so the
// generated docs stay stable and match the schema author's intended reading order.
func orderedPropertyNames(schema *jsonschema.Schema) ([]string, error) {
	if len(schema.Properties) == 1 {
		for name := range schema.Properties {
			return []string{name}, nil
		}
	}

	seen := make(map[string]bool, len(schema.Properties))
	order := docOrder(schema)
	if len(order) == 0 {
		return nil, fmt.Errorf("%s: missing x-doc-order for object with multiple properties", schemaLocation(schema))
	}

	names := make([]string, 0, len(schema.Properties))
	for _, name := range order {
		if _, ok := schema.Properties[name]; ok {
			names = append(names, name)
			seen[name] = true
			continue
		}
		return nil, fmt.Errorf("%s: x-doc-order references unknown property %q", schemaLocation(schema), name)
	}

	for name := range schema.Properties {
		if !seen[name] {
			return nil, fmt.Errorf("%s: property %q missing from x-doc-order", schemaLocation(schema), name)
		}
	}
	return names, nil
}

// renderParam reduces one schema node to the doc shape needed by markdown output.
// It keeps the property wrapper description, then inspects the resolved schema to
// infer type, nested fields, and enum details.
func renderParam(name string, schema *jsonschema.Schema, required bool) (paramDoc, error) {
	resolved := resolveSchema(schema)
	if resolved == nil {
		return paramDoc{Name: name, Type: "value", Required: required}, nil
	}

	out := paramDoc{
		Name:        name,
		Type:        describeSchema(resolved),
		Required:    required,
		Description: schema.Description,
	}

	if isRefObjectSchema(schema) {
		out.Type = "reference"
		return out, nil
	}
	if applyArrayDetails(&out, resolved) {
		return out, nil
	}
	if enumVals := enumValues(resolved); len(enumVals) > 0 {
		out.EnumValues = enumVals
	}
	if len(resolved.Properties) > 0 {
		children, err := renderProperties(resolved)
		if err != nil {
			return paramDoc{}, err
		}
		out.Children = children
	}

	return out, nil
}

// applyArrayDetails special-cases array item shapes that read more clearly than
// a plain "array" in the generated reference.
func applyArrayDetails(out *paramDoc, schema *jsonschema.Schema) bool {
	if schema == nil || schema.Items2020 == nil {
		return false
	}
	item := schema.Items2020
	if item == nil {
		return false
	}
	if isRefObjectSchema(item) {
		out.Type = "array of references"
		return true
	}
	if enumVals := enumValues(item); len(enumVals) > 0 {
		out.Type = "array of strings"
		out.EnumValues = enumVals
		return true
	}
	return false
}

// describeSchema maps the compiled schema shape to the short type labels shown
// next to each parameter in markdown.
func describeSchema(schema *jsonschema.Schema) string {
	if schema == nil {
		return "value"
	}
	if isRefObjectSchema(schema) {
		return "reference"
	}
	if schema.Types != nil {
		types := schema.Types.ToStrings()
		if len(types) == 1 {
			return types[0]
		}
		if len(types) > 1 {
			return strings.Join(types, " | ")
		}
	}
	if item := schema.Items2020; item != nil {
		if isRefObjectSchema(item) {
			return "array of references"
		}
		if enumValues(item) != nil {
			return "array of strings"
		}
		return "array"
	}
	if len(schema.Properties) > 0 {
		return "object"
	}
	if schema.Enum != nil {
		return inferValueType(schema.Enum.Values)
	}
	if schema.Const != nil {
		return inferValueType([]any{*schema.Const})
	}
	return "value"
}

func enumValues(schema *jsonschema.Schema) []string {
	if schema == nil || schema.Enum == nil {
		return nil
	}
	out := make([]string, 0, len(schema.Enum.Values))
	for _, value := range schema.Enum.Values {
		if s, ok := value.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func enumLeadIn(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ": Allowed values:"
	}
	switch desc[len(desc)-1] {
	case '.', ':', ';':
		return " Allowed values:"
	default:
		return ". Allowed values:"
	}
}

func inferValueType(values []any) string {
	if len(values) == 0 {
		return "value"
	}
	switch values[0].(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return "value"
	}
}

func oneOfSchemas(schema *jsonschema.Schema) []*jsonschema.Schema {
	schema = resolveSchema(schema)
	if schema == nil {
		return nil
	}
	return schema.OneOf
}

// Placeholder text used when schema descriptions are absent.
func fallbackResultText(schema *jsonschema.Schema) string {
	if schemaKind(schema) == kindNullResponse {
		return "`null` (void operation)"
	}
	return missingSchemaDescription("result", schema)
}

func responseTypeLabel(schema *jsonschema.Schema) string {
	switch schemaKind(schema) {
	case kindRefResponse:
		return "Reference"
	case kindNullResponse:
		return "Null"
	case kindBooleanResponse:
		return "Boolean"
	case kindIntegerResponse:
		return "Integer"
	case kindHexStringResponse, kindHex64StringResponse:
		return "String"
	default:
		return ""
	}
}

func fallbackErrorText(schema *jsonschema.Schema) string {
	if schemaKind(schema) == kindGenericErrorResponse {
		return "`{}` when operation fails"
	}
	return missingSchemaDescription("error", schema)
}

func missingSchemaDescription(kind string, schema *jsonschema.Schema) string {
	loc := schemaLocation(schema)
	if loc == "<unknown schema>" {
		return fmt.Sprintf("TODO: add %s schema description", kind)
	}
	return fmt.Sprintf("TODO: add %s schema description (%s)", kind, loc)
}

func isRefObjectSchema(schema *jsonschema.Schema) bool {
	return schemaKind(schema) == kindRefObject
}

// isErrorResponseSchema treats any response with a non-null `error` field as an error envelope.
func isErrorResponseSchema(schema *jsonschema.Schema) bool {
	if schemaKind(schema) == kindGenericErrorResponse {
		return true
	}
	resolved := resolveSchema(schema)
	if resolved == nil {
		return false
	}
	errorSchema := resolveSchema(resolved.Properties["error"])
	if errorSchema == nil {
		return false
	}
	// Treat any non-null error schema as an error response, including untyped `{}`.
	if errorSchema.Types == nil {
		return true
	}
	types := errorSchema.Types.ToStrings()
	return !(len(types) == 1 && types[0] == "null")
}

func schemaDescription(schema *jsonschema.Schema) string {
	original := schema
	for schema != nil {
		if schema.Description != "" {
			desc := strings.TrimSpace(schema.Description)
			if desc == "" {
				return ""
			}
			if label := responseTypeLabel(original); label != "" {
				return label + " - " + desc
			}
			return desc
		}
		schema = schema.Ref
	}
	return ""
}

func schemaLocation(schema *jsonschema.Schema) string {
	resolved := resolveSchema(schema)
	if resolved == nil || resolved.Location == "" {
		return "<unknown schema>"
	}
	return resolved.Location
}

// Match known response/reference kinds from shared schema definition paths.
func schemaKind(schema *jsonschema.Schema) string {
	if schema == nil {
		return ""
	}
	// Classify known response/reference shapes based on resolved $defs location.
	resolved := resolveSchema(schema)
	if resolved == nil {
		return ""
	}
	loc := resolved.Location
	switch {
	case strings.Contains(loc, "#/$defs/RefObject"):
		return kindRefObject
	case strings.Contains(loc, "#/$defs/GenericErrorResponse"):
		return kindGenericErrorResponse
	case strings.Contains(loc, "#/$defs/RefResponse"):
		return kindRefResponse
	case strings.Contains(loc, "#/$defs/NullResponse"):
		return kindNullResponse
	case strings.Contains(loc, "#/$defs/BooleanResponse"):
		return kindBooleanResponse
	case strings.Contains(loc, "#/$defs/IntegerResponse"):
		return kindIntegerResponse
	case strings.Contains(loc, "#/$defs/HexStringResponse"):
		return kindHexStringResponse
	case strings.Contains(loc, "#/$defs/Hex64StringResponse"):
		return kindHex64StringResponse
	default:
		return ""
	}
}

// schemaAt walks a simple property path without resolving refs. It is used for
// fixed top-level locations like request.method and expected_response.
func schemaAt(schema *jsonschema.Schema, parts ...string) *jsonschema.Schema {
	cur := schema
	for _, part := range parts {
		if cur == nil {
			return nil
		}
		next, ok := cur.Properties[part]
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func stringConst(schema *jsonschema.Schema) (string, error) {
	if schema == nil || schema.Const == nil {
		return "", errors.New("const missing")
	}
	s, ok := (*schema.Const).(string)
	if !ok {
		return "", errors.New("const is not a string")
	}
	return s, nil
}

func markdownSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	switch s[len(s)-1] {
	case '.', '!', '?', ')':
		return s
	default:
		return s + "."
	}
}
