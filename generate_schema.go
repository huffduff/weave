package weave

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Constants for tag and marker identification
const (
	weaviateTag = "weave" // Custom struct tag for Weaviate

	// Comment markers
	weaviateMarker       = "+" + weaviateTag              // Marks a struct to be included in Weaviate schema
	weaviateDescMarker   = "+" + weaviateTag + ":desc:"   // Provides a description for the Weaviate class
	weaviateConfigMarker = "+" + weaviateTag + ":config:" // Provides configuration for the Weaviate class
)

// WeaviateClass represents a Weaviate class schema definition
type WeaviateClass struct {
	Package             string                 `json:"-"`
	Class               string                 `json:"class"`
	Description         string                 `json:"description,omitempty"`
	VectorIndexType     string                 `json:"vectorIndexType,omitempty"`
	VectorIndexConfig   map[string]interface{} `json:"vectorIndexConfig,omitempty"`
	Properties          []WeaviateProperty     `json:"properties"`
	Vectorizer          string                 `json:"vectorizer,omitempty"`
	ModuleConfig        map[string]interface{} `json:"moduleConfig,omitempty"`
	ShardingConfig      map[string]interface{} `json:"shardingConfig,omitempty"`
	ReplicationConfig   map[string]interface{} `json:"replicationConfig,omitempty"`
	InvertedIndexConfig map[string]interface{} `json:"invertedIndexConfig,omitempty"`
}

// WeaviateProperty represents a property in a Weaviate class
type WeaviateProperty struct {
	Name            string   `json:"name"`
	DataType        []string `json:"dataType"`
	Description     string   `json:"description,omitempty"`
	Tokenization    string   `json:"tokenization,omitempty"`
	IndexFilterable bool     `json:"indexFilterable,omitempty"`
	IndexSearchable bool     `json:"indexSearchable,omitempty"`
	IndexInverted   bool     `json:"indexInverted,omitempty"`
}

// WeaviateSchemaDefinition represents the entire schema
type WeaviateSchemaDefinition struct {
	Classes []WeaviateClass `json:"classes"`
}

// ToJSON converts the schema to a JSON string
func (s *WeaviateSchemaDefinition) ToJSON(pretty bool) ([]byte, error) {
	if pretty {
		return json.MarshalIndent(s, "", "  ")
	}
	return json.Marshal(s)
}

// GenerateWeaviateSchema processes Go source files and generates Weaviate schema
func GenerateWeaviateSchema(srcDir string) (*WeaviateSchemaDefinition, error) {
	schema := &WeaviateSchemaDefinition{
		Classes: []WeaviateClass{},
	}

	// Set up the file set
	fset := token.NewFileSet()

	// Process files in the directory
	err := processGoFiles(srcDir, fset, schema)
	if err != nil {
		return nil, err
	}

	return schema, nil
}

// processGoFiles processes Go files in a directory
func processGoFiles(dir string, fset *token.FileSet, schema *WeaviateSchemaDefinition) error {
	// Read the directory
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return fmt.Errorf("error reading directory %s: %v", dir, err)
	}

	// Process each file/directory
	for _, path := range files {
		// Parse the Go file
		goFile, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("error parsing file %s: %v", path, err)
		}

		// Process the file's AST to find structs
		if err := processFileAST(goFile, fset, schema); err != nil {
			return err
		}
	}

	return nil
}

// processStruct converts a Go struct into a Weaviate class
func processStruct(packageName, structName string, structType *ast.StructType) (*WeaviateClass, error) {
	class := &WeaviateClass{
		Package:    packageName,
		Class:      structName,
		Properties: []WeaviateProperty{},
		// Set default values for Weaviate schema
		VectorIndexType: "hnsw",
		Vectorizer:      "text2vec-contextionary",
	}

	// Process each field in the struct
	for _, field := range structType.Fields.List {
		// Skip embedded or unnamed fields
		if len(field.Names) == 0 {
			continue
		}

		fieldName := field.Names[0].Name

		// Skip unexported fields (starting with lowercase letter)
		if !ast.IsExported(fieldName) {
			continue
		}

		// Process field tags
		var propName string
		var weaviateConfig map[string]string

		if field.Tag != nil {
			tagValue, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				return nil, fmt.Errorf("error unquoting struct tag: %v", err)
			}

			propName = extractJSONFieldName(tagValue)
			if propName == "-" {
				continue
			}

			weaviateConfig = extractWeaviateConfig(tagValue)
		}

		// Use JSON name if available, otherwise use field name
		if propName == "" {
			propName = strings.ToLower(fieldName[:1]) + fieldName[1:]
		}

		var dataType []string
		if weaviateConfig != nil {
			if dt, ok := weaviateConfig["type"]; ok {
				dataType = []string{dt}
				delete(weaviateConfig, "type")
			}
		}

		// Determine the data type
		if dataType == nil {
			d, err := determineWeaviateDataType(field.Type)
			if err != nil {
				return nil, fmt.Errorf("error determining data type for field %s: %v", fieldName, err)
			}
			dataType = d
		}

		// Create the property
		property := WeaviateProperty{
			Name:     propName,
			DataType: dataType,
		}

		// Apply Weaviate-specific configurations from tags
		if desc, ok := weaviateConfig["description"]; ok {
			property.Description = desc
		}

		if tokenization, ok := weaviateConfig["tokenization"]; ok {
			property.Tokenization = tokenization
		}

		if val, ok := weaviateConfig["indexFilterable"]; ok {
			property.IndexFilterable = val == "true"
		}

		if val, ok := weaviateConfig["indexSearchable"]; ok {
			property.IndexSearchable = val == "true"
		}

		if val, ok := weaviateConfig["indexInverted"]; ok {
			property.IndexInverted = val == "true"
		}

		class.Properties = append(class.Properties, property)
	}

	return class, nil
}

// extractJSONFieldName extracts the field name from the json tag
func extractJSONFieldName(tagValue string) string {
	tags := reflect.StructTag(tagValue)
	jsonTag := tags.Get("json")
	if jsonTag == "" {
		return ""
	}

	parts := strings.Split(jsonTag, ",")
	return parts[0]
}

// extractWeaviateConfig extracts Weaviate-specific configurations from the struct tag
func extractWeaviateConfig(tagValue string) map[string]string {
	config := make(map[string]string)
	tags := reflect.StructTag(tagValue)

	tag := tags.Get(weaviateTag)
	if tag == "" {
		return config
	}

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		keyVal := strings.Split(part, "=")
		if len(keyVal) == 2 {
			config[keyVal[0]] = keyVal[1]
		}
	}

	return config
}

// determineWeaviateDataType maps Go types to Weaviate data types
func determineWeaviateDataType(expr ast.Expr) ([]string, error) {
	switch t := expr.(type) {
	case *ast.Ident:
		// Basic types
		switch t.Name {
		case "string":
			return []string{"text"}, nil
		// FIXME warn on uint types that won't fit in an int64?
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
			return []string{"int"}, nil
		case "float16", "float32", "float64":
			return []string{"number"}, nil
		case "bool":
			return []string{"boolean"}, nil
		}

		// Could be a custom type, enum, or reference to another class
		if ast.IsExported(t.Name) {
			// Likely a reference to another class
			return []string{t.Name}, nil
		}
		return []string{"text"}, nil

	case *ast.ArrayType:
		// Array or slice type
		elemType, err := determineWeaviateDataType(t.Elt)
		if err != nil {
			return nil, err
		}

		// not all types are array compatible
		nativeTypes := []string{"text", "boolean", "int", "number", "date", "uuid", "object"}

		// For arrays of primitives in Weaviate, we need to indicate the array type
		if len(elemType) == 1 && slices.Contains(nativeTypes, elemType[0]) {
			return []string{elemType[0] + "[]"}, nil
		}

		if len(elemType) == 1 && ast.IsExported(elemType[0]) {
			// assume it's a reference to another type, which is always an array but represented directly in the schema
			return []string{elemType[0]}, nil
		}

		// Default to "object[]" for other types, since Weaviate doesn't support arrays of complex types
		return []string{"object[]"}, nil

	case *ast.StarExpr:
		// Pointer type
		return determineWeaviateDataType(t.X)

	case *ast.SelectorExpr:
		// Qualified identifier (e.g., time.Time)
		if ident, ok := t.X.(*ast.Ident); ok {
			if ident.Name == "time" && t.Sel.Name == "Time" {
				return []string{"date"}, nil
			}
			if ident.Name == "uuid" && t.Sel.Name == "UUID" {
				return []string{"uuid"}, nil
			}
		}

		// Default to "string" for other external types
		return []string{"text"}, nil

	case *ast.StructType:
		// Embedded struct - use "object" type in Weaviate
		return []string{"object"}, nil

	case *ast.MapType:
		// Map type - typically represented as "object" in Weaviate
		return []string{"object"}, nil

	case *ast.InterfaceType:
		// Interface{} type - can be any type
		return []string{"text"}, nil
	}

	return nil, fmt.Errorf("unsupported type: %T", expr)
}

// processFileAST processes the AST of a Go file to extract struct information for Weaviate schema
func processFileAST(file *ast.File, fset *token.FileSet, schema *WeaviateSchemaDefinition) error {
	packageName := file.Name.Name

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue // Skip non-type declarations
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			// Process struct type
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			// Check for marker comment that indicates this struct should be included in Weaviate
			includeInWeaviate := hasWeaviateMarker(genDecl.Doc)
			if !includeInWeaviate {
				// If no doc comment on GenDecl, check TypeSpec's comment
				includeInWeaviate = hasWeaviateMarker(typeSpec.Doc)
			}

			if !includeInWeaviate {
				continue
			}

			// Extract description and config
			description := extractWeaviateDescription(genDecl.Doc)
			if description == "" {
				description = extractWeaviateDescription(typeSpec.Doc)
			}

			config := extractWeaviateClassConfig(genDecl.Doc)
			if len(config) == 0 {
				config = extractWeaviateClassConfig(typeSpec.Doc)
			}

			// Process the struct into a Weaviate class
			class, err := processStruct(packageName, typeSpec.Name.Name, structType)
			if err != nil {
				return fmt.Errorf("error processing struct %s: %v", typeSpec.Name.Name, err)
			}

			// Add description and config
			if description != "" {
				class.Description = description
			}

			// Apply configuration
			applyClassConfig(class, config)

			schema.Classes = append(schema.Classes, *class)
		}
	}

	return nil
}

// hasWeaviateMarker checks if the comment group contains a marker like "+weave"
func hasWeaviateMarker(cg *ast.CommentGroup) bool {
	if cg == nil {
		return false
	}

	for _, c := range cg.List {
		// Look for "+weave" comment marker
		if strings.Contains(c.Text, weaviateMarker) {
			return true
		}
	}

	return false
}

// extractWeaviateDescription extracts the class description from comments
func extractWeaviateDescription(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}

	for _, c := range cg.List {
		if strings.Contains(c.Text, weaviateDescMarker) {
			// Extract the description that follows the marker
			parts := strings.SplitN(c.Text, weaviateDescMarker, 2)
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	return ""
}

// extractWeaviateClassConfig extracts class-level configuration from comments
func extractWeaviateClassConfig(cg *ast.CommentGroup) map[string]interface{} {
	config := make(map[string]interface{})

	if cg == nil {
		return config
	}

	for _, c := range cg.List {
		if strings.Contains(c.Text, weaviateConfigMarker) {
			// Extract the configuration that follows the marker
			parts := strings.SplitN(c.Text, weaviateConfigMarker, 2)
			if len(parts) > 1 {
				configStr := strings.TrimSpace(parts[1])

				// Parse the configuration string
				// Format: key1=value1;key2=value2;...
				configParts := strings.Split(configStr, ";")
				for _, part := range configParts {
					if part == "" {
						continue
					}

					keyVal := strings.SplitN(part, "=", 2)
					if len(keyVal) == 2 {
						key := strings.TrimSpace(keyVal[0])
						value := strings.TrimSpace(keyVal[1])

						// Try to convert value to appropriate type
						if val, err := strconv.ParseBool(value); err == nil {
							config[key] = val
						} else if val, err := strconv.ParseFloat(value, 64); err == nil {
							config[key] = val
						} else if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
							// Try to parse as JSON object
							var jsonVal map[string]interface{}
							if err := json.Unmarshal([]byte(value), &jsonVal); err == nil {
								config[key] = jsonVal
							} else {
								config[key] = value
							}
						} else {
							config[key] = value
						}
					}
				}
			}
		}
	}

	return config
}

// applyClassConfig applies configuration to a Weaviate class
func applyClassConfig(class *WeaviateClass, config map[string]interface{}) {
	for key, value := range config {
		switch key {
		case "vectorIndexType":
			if strValue, ok := value.(string); ok {
				class.VectorIndexType = strValue
			}
		case "vectorizer":
			if strValue, ok := value.(string); ok {
				class.Vectorizer = strValue
			}
		case "vectorIndexConfig":
			if mapValue, ok := value.(map[string]interface{}); ok {
				class.VectorIndexConfig = mapValue
			}
		case "moduleConfig":
			if mapValue, ok := value.(map[string]interface{}); ok {
				class.ModuleConfig = mapValue
			}
		case "shardingConfig":
			if mapValue, ok := value.(map[string]interface{}); ok {
				class.ShardingConfig = mapValue
			}
		case "replicationConfig":
			if mapValue, ok := value.(map[string]interface{}); ok {
				class.ReplicationConfig = mapValue
			}
		case "invertedIndexConfig":
			if mapValue, ok := value.(map[string]interface{}); ok {
				class.InvertedIndexConfig = mapValue
			}
		}
	}
}
