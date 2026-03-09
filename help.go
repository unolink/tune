package tune

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FieldInfo holds metadata about a configuration field.
type FieldInfo struct {
	Name           string
	Type           string
	EnvVar         string
	YAMLKey        string
	Default        string
	Usage          string
	HotReload      string
	IndentLevel    int
	IsSecret       bool
	IsNested       bool
	IsArrayElement bool
}

// DocField holds structured documentation about a configuration field.
// Intended for programmatic access to field metadata.
type DocField struct {
	Field          string `json:"field" yaml:"field"`
	Type           string `json:"type" yaml:"type"`
	ENV            string `json:"env" yaml:"env"`
	YAML           string `json:"yaml" yaml:"yaml"`
	Default        string `json:"default" yaml:"default"`
	Usage          string `json:"usage,omitempty" yaml:"usage,omitempty"`
	HotReload      string `json:"hot_reload,omitempty" yaml:"hot_reload,omitempty"`
	IsSecret       bool   `json:"is_secret,omitempty" yaml:"is_secret,omitempty"`
	IsArrayElement bool   `json:"is_array_element,omitempty" yaml:"is_array_element,omitempty"`
}

// DocSection holds structured documentation about a configuration section.
// Intended for programmatic access to section metadata.
type DocSection struct {
	// Key is the section key (e.g., "logger", "server").
	Key string `json:"key" yaml:"key"`
	// ENVPrefix is the ENV variable prefix for this section (e.g., "MYAPP_SERVER_").
	ENVPrefix string `json:"env_prefix" yaml:"env_prefix"`
	// Fields is the list of fields in the section.
	Fields []DocField `json:"fields" yaml:"fields"`
}

// GetUsage generates a CLI-friendly help message describing all registered configurations.
// Returns a formatted string with information about each field: type, ENV variable, YAML key, default, and description.
func (m *Manager) GetUsage() string {
	m.mu.RLock()
	if len(m.sections) == 0 {
		m.mu.RUnlock()
		return "No configuration sections registered."
	}

	// Copy sections to work without holding the lock.
	sectionsCopy := make(map[string]Section, len(m.sections))
	for k, v := range m.sections {
		sectionsCopy[k] = v
	}
	m.mu.RUnlock()

	var builder strings.Builder
	builder.WriteString("Configuration Reference\n")
	builder.WriteString(strings.Repeat("=", 80) + "\n\n")

	keys := make([]string, 0, len(sectionsCopy))
	for key := range sectionsCopy {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		section, ok := sectionsCopy[key]
		if !ok {
			continue
		}
		sectionInfo := m.analyzeSection(section)
		m.formatSectionUsage(&builder, key, sectionInfo)
	}

	return builder.String()
}

// GetDocumentation returns structured documentation about all registered configuration sections.
// Allows consumers to format the output themselves (HTML, terminal tables, etc.).
//
// Example usage:
//
//	docs := cfgManager.GetDocumentation()
//	for _, section := range docs {
//	    fmt.Printf("Section: %s\n", section.Key)
//	    for _, field := range section.Fields {
//	        fmt.Printf("  %s: %s (default: %s)\n", field.Field, field.Type, field.Default)
//	    }
//	}
func (m *Manager) GetDocumentation() []DocSection {
	m.mu.RLock()
	if len(m.sections) == 0 {
		m.mu.RUnlock()
		return nil
	}

	// Copy sections to work without holding the lock.
	sectionsCopy := make(map[string]Section, len(m.sections))
	for k, v := range m.sections {
		sectionsCopy[k] = v
	}
	m.mu.RUnlock()

	keys := make([]string, 0, len(sectionsCopy))
	for key := range sectionsCopy {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sections []DocSection
	for _, key := range keys {
		section, ok := sectionsCopy[key]
		if !ok {
			continue
		}
		sectionDoc := m.buildSectionDocumentation(key, section)
		sections = append(sections, sectionDoc)
	}

	return sections
}

// buildSectionDocumentation builds structured documentation for a section.
func (m *Manager) buildSectionDocumentation(sectionKey string, section Section) DocSection {
	fieldInfos := m.analyzeSection(section)

	fields := make([]DocField, 0, len(fieldInfos))
	for i := range fieldInfos {
		hotReload := fieldInfos[i].HotReload
		if hotReload == "" {
			hotReload = "no"
		}

		field := DocField{
			Field:          fieldInfos[i].Name,
			Type:           fieldInfos[i].Type,
			ENV:            fieldInfos[i].EnvVar,
			YAML:           fieldInfos[i].YAMLKey,
			Default:        fieldInfos[i].Default,
			Usage:          fieldInfos[i].Usage,
			IsSecret:       fieldInfos[i].IsSecret,
			IsArrayElement: fieldInfos[i].IsArrayElement,
			HotReload:      hotReload,
		}
		fields = append(fields, field)
	}

	prefix := envPrefix(m.globalPrefix, sectionKey)

	return DocSection{
		Key:       sectionKey,
		ENVPrefix: prefix,
		Fields:    fields,
	}
}

// analyzeSection analyzes a section and returns information about all its fields.
// Results are cached since struct metadata does not change at runtime.
func (m *Manager) analyzeSection(section Section) []FieldInfo {
	sectionKey := section.ConfigKey()

	m.mu.RLock()
	if cached, ok := m.metadataCache[sectionKey]; ok {
		m.mu.RUnlock()
		return cached
	}
	m.mu.RUnlock()

	sectionType := reflect.TypeOf(section).Elem()
	newSection := reflect.New(sectionType).Interface().(Section)
	newSection.SetDefaults()

	val := reflect.ValueOf(newSection).Elem()
	prefix := envPrefix(m.globalPrefix, sectionKey)

	var fields []FieldInfo
	m.collectFieldInfo(val, sectionType, prefix, "", 0, &fields)

	m.mu.Lock()
	m.metadataCache[sectionKey] = fields
	m.mu.Unlock()

	return fields
}

// collectFieldInfo recursively collects information about struct fields.
func (m *Manager) collectFieldInfo(val reflect.Value, typ reflect.Type, envPrefix, yamlPrefix string, indentLevel int, fields *[]FieldInfo) {
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		typeField := typ.Field(i)

		if !field.CanSet() {
			continue
		}

		yamlTag := typeField.Tag.Get("yaml")
		if strings.HasPrefix(yamlTag, "-") {
			continue
		}

		yamlKey := extractYAMLKey(yamlTag, typeField.Name)
		if yamlPrefix != "" {
			yamlKey = yamlPrefix + "." + yamlKey
		}

		usage := typeField.Tag.Get("usage")

		secretTag := typeField.Tag.Get("secret")
		isSecret := secretTag == "true"

		hotReloadTag := typeField.Tag.Get("hotreload")
		// Valid values: "yes", "restart", "no" (empty tag = no info).

		envVar := envPrefix + strings.ToUpper(typeField.Name)

		typeStr := getTypeString(field.Type())

		defaultValue := getDefaultValueString(field, typeField.Type)

		if field.Kind() == reflect.Struct && field.Type() != reflect.TypeOf(time.Duration(0)) {
			*fields = append(*fields, FieldInfo{
				Name:           typeField.Name,
				Type:           typeStr,
				EnvVar:         envVar,
				YAMLKey:        yamlKey,
				Default:        "<nested struct>",
				Usage:          usage,
				IsSecret:       isSecret,
				IsNested:       true,
				IsArrayElement: false,
				HotReload:      hotReloadTag,
				IndentLevel:    indentLevel,
			})

			m.collectFieldInfo(field, field.Type(), envPrefix+strings.ToUpper(typeField.Name)+"_", yamlKey, indentLevel+1, fields)
			continue
		}

		if field.Kind() == reflect.Slice {
			elemType := field.Type().Elem()
			if elemType.Kind() == reflect.Struct {
				*fields = append(*fields, FieldInfo{
					Name:           typeField.Name,
					Type:           typeStr,
					EnvVar:         envVar,
					YAMLKey:        yamlKey,
					Default:        "[]",
					Usage:          usage + " (JSON in ENV or YAML array)",
					IsSecret:       isSecret,
					IsNested:       true,
					IsArrayElement: false,
					HotReload:      hotReloadTag,
					IndentLevel:    indentLevel,
				})

				// Create a temp instance of the element type so we can
				// show element fields even when the slice is empty.
				tempElem := reflect.New(elemType).Elem()

				fieldsBeforeAnalysis := len(*fields)

				// Analyze element structure without ENV prefix since elements are specified via JSON.
				m.collectFieldInfo(tempElem, elemType, "", yamlKey+"[]", indentLevel+1, fields)

				for i := fieldsBeforeAnalysis; i < len(*fields); i++ {
					(*fields)[i].IsArrayElement = true
				}
				continue
			}
		}

		*fields = append(*fields, FieldInfo{
			Name:           typeField.Name,
			Type:           typeStr,
			EnvVar:         envVar,
			YAMLKey:        yamlKey,
			Default:        defaultValue,
			Usage:          usage,
			IsSecret:       isSecret,
			IsNested:       false,
			IsArrayElement: false,
			HotReload:      hotReloadTag,
			IndentLevel:    indentLevel,
		})
	}
}

// formatSectionUsage formats section information for output.
func (m *Manager) formatSectionUsage(builder *strings.Builder, sectionKey string, fields []FieldInfo) {
	prefix := envPrefix(m.globalPrefix, sectionKey)
	fmt.Fprintf(builder, "SECTION: %s (ENV Prefix: %s)\n", sectionKey, prefix)
	builder.WriteString(strings.Repeat("-", 80) + "\n")

	for i := range fields {
		indent := strings.Repeat("  ", fields[i].IndentLevel)
		fmt.Fprintf(builder, "%sField:        %s\n", indent, fields[i].Name)
		fmt.Fprintf(builder, "%sType:         %s\n", indent, fields[i].Type)
		fmt.Fprintf(builder, "%sENV:          %s\n", indent, fields[i].EnvVar)
		fmt.Fprintf(builder, "%sYAML:         %s\n", indent, fields[i].YAMLKey)
		fmt.Fprintf(builder, "%sDefault:      %s\n", indent, fields[i].Default)
		if fields[i].Usage != "" {
			fmt.Fprintf(builder, "%sUsage:        %s\n", indent, fields[i].Usage)
		}
		if fields[i].IsSecret {
			fmt.Fprintf(builder, "%sSecret:       true\n", indent)
		}
		if fields[i].HotReload != "" {
			fmt.Fprintf(builder, "%sHot-Reload:   %s\n", indent, fields[i].HotReload)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

// GetDebugConfigYAML returns the current configuration as YAML with secrets masked.
// Secrets are identified by the secret:"true" tag and replaced with "<REDACTED>".
// Does not modify the actual configuration — masking is applied only to the output string.
func (m *Manager) GetDebugConfigYAML() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.sections) == 0 {
		return "# No configuration sections registered.\n", nil
	}

	maskedConfig := make(map[string]any)

	keys := make([]string, 0, len(m.sections))
	for key := range m.sections {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		section := m.sections[key]
		maskedSection := m.maskSecrets(section)
		maskedConfig[key] = maskedSection
	}

	yamlBytes, err := yaml.Marshal(maskedConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// maskSecrets creates a copy of the section with secrets masked.
func (m *Manager) maskSecrets(section Section) any {
	val := reflect.ValueOf(section)
	if val.Kind() != reflect.Ptr {
		return nil
	}

	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return nil
	}

	typ := val.Type()
	masked := make(map[string]any)

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		typeField := typ.Field(i)

		if !field.CanSet() {
			continue
		}

		yamlTag := typeField.Tag.Get("yaml")
		if strings.HasPrefix(yamlTag, "-") {
			continue
		}

		yamlKey := extractYAMLKey(yamlTag, typeField.Name)
		secretTag := typeField.Tag.Get("secret")

		if secretTag == "true" {
			masked[yamlKey] = "<REDACTED>"
			continue
		}

		maskedValue := m.maskSecretsRecursive(field, typeField.Type)
		masked[yamlKey] = maskedValue
	}

	return masked
}

// maskSecretsRecursive recursively masks secrets in values.
func (m *Manager) maskSecretsRecursive(val reflect.Value, typ reflect.Type) any {
	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return nil
		}
		return m.maskSecretsRecursive(val.Elem(), typ.Elem())

	case reflect.Struct:
		if typ == reflect.TypeOf(time.Duration(0)) {
			return val.Interface()
		}

		result := make(map[string]any)
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			typeField := typ.Field(i)

			if !field.CanSet() {
				continue
			}

			yamlTag := typeField.Tag.Get("yaml")
			if strings.HasPrefix(yamlTag, "-") {
				continue
			}

			yamlKey := extractYAMLKey(yamlTag, typeField.Name)
			secretTag := typeField.Tag.Get("secret")

			if secretTag == "true" {
				result[yamlKey] = "<REDACTED>"
				continue
			}

			maskedValue := m.maskSecretsRecursive(field, typeField.Type)
			result[yamlKey] = maskedValue
		}
		return result

	case reflect.Slice, reflect.Array:
		if val.Len() == 0 {
			return []any{}
		}

		result := make([]any, val.Len())
		for i := 0; i < val.Len(); i++ {
			elem := val.Index(i)
			result[i] = m.maskSecretsRecursive(elem, typ.Elem())
		}
		return result

	case reflect.Map:
		result := make(map[string]any)
		for _, key := range val.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			value := val.MapIndex(key)
			result[keyStr] = m.maskSecretsRecursive(value, typ.Elem())
		}
		return result

	default:
		return val.Interface()
	}
}

// extractYAMLKey extracts the YAML key from a struct tag.
func extractYAMLKey(yamlTag, fieldName string) string {
	if yamlTag == "" || yamlTag == "-" {
		return strings.ToLower(fieldName)
	}

	// Uses strings.Cut (Go 1.18+) to avoid a slice allocation from strings.Split.
	key, _, _ := strings.Cut(yamlTag, ",")
	return key
}

// getTypeString returns a human-readable string representation of a reflect.Type.
func getTypeString(typ reflect.Type) string {
	switch typ.Kind() {
	case reflect.Ptr:
		return "*" + getTypeString(typ.Elem())
	case reflect.Slice:
		return "[]" + getTypeString(typ.Elem())
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", typ.Len(), getTypeString(typ.Elem()))
	case reflect.Map:
		return fmt.Sprintf("map[%s]%s", getTypeString(typ.Key()), getTypeString(typ.Elem()))
	case reflect.Struct:
		if typ == reflect.TypeOf(time.Duration(0)) {
			return "time.Duration"
		}
		return typ.Name()
	default:
		return typ.Kind().String()
	}
}

// getDefaultValueString returns a human-readable string representation of a default value.
func getDefaultValueString(val reflect.Value, typ reflect.Type) string {
	if !val.IsValid() {
		return "<nil>"
	}

	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return "<nil>"
		}
		return getDefaultValueString(val.Elem(), typ.Elem())

	case reflect.String:
		if val.String() == "" {
			return `""`
		}
		return fmt.Sprintf("%q", val.String())

	case reflect.Bool:
		return fmt.Sprintf("%v", val.Bool())

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if typ == reflect.TypeOf(time.Duration(0)) {
			return val.Interface().(time.Duration).String()
		}
		return fmt.Sprintf("%d", val.Int())

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", val.Uint())

	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", val.Float())

	case reflect.Slice, reflect.Array:
		if val.Len() == 0 {
			return "[]"
		}
		return fmt.Sprintf("[%d items]", val.Len())

	case reflect.Struct:
		if typ == reflect.TypeOf(time.Duration(0)) {
			return val.Interface().(time.Duration).String()
		}
		return "<struct>"

	case reflect.Map:
		if val.Len() == 0 {
			return "{}"
		}
		return fmt.Sprintf("{%d items}", val.Len())

	default:
		return fmt.Sprintf("%v", val.Interface())
	}
}

// GetDefaultConfigYAML generates a valid YAML file with the entire configuration structure
// populated with default values. Enables users to create a config file template
// via a CLI flag (e.g., --init-config).
//
// Creates new struct instances for each section, calls SetDefaults(),
// and marshals the result to YAML. Does not modify the state of registered sections.
func (m *Manager) GetDefaultConfigYAML() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.sections) == 0 {
		return []byte("# No configuration sections registered.\n"), nil
	}

	defaultConfig := make(map[string]any)

	keys := make([]string, 0, len(m.sections))
	for key := range m.sections {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		section := m.sections[key]

		sectionType := reflect.TypeOf(section).Elem()
		newSection := reflect.New(sectionType).Interface().(Section)

		newSection.SetDefaults()

		sectionMap := m.structToMap(reflect.ValueOf(newSection).Elem())

		defaultConfig[key] = sectionMap
	}

	yamlBytes, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal default config to YAML: %w", err)
	}

	return yamlBytes, nil
}

// structToMap converts a struct to map[string]any for YAML marshaling.
// Uses yaml tags to determine keys.
func (m *Manager) structToMap(val reflect.Value) map[string]any {
	if val.Kind() != reflect.Struct {
		return nil
	}

	result := make(map[string]any)
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		typeField := typ.Field(i)

		if !field.CanSet() {
			continue
		}

		yamlTag := typeField.Tag.Get("yaml")
		if strings.HasPrefix(yamlTag, "-") {
			continue
		}

		yamlKey := extractYAMLKey(yamlTag, typeField.Name)

		fieldValue := m.valueToInterface(field, typeField.Type)
		result[yamlKey] = fieldValue
	}

	return result
}

// valueToInterface converts a reflect.Value to any for marshaling.
// Recursively handles nested structs, slices, maps, and pointers.
func (m *Manager) valueToInterface(val reflect.Value, typ reflect.Type) any {
	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return nil
		}
		return m.valueToInterface(val.Elem(), typ.Elem())

	case reflect.Struct:
		if typ == reflect.TypeOf(time.Duration(0)) {
			return val.Interface().(time.Duration).String()
		}
		return m.structToMap(val)

	case reflect.Slice, reflect.Array:
		elemType := typ.Elem()

		// For empty slices of structs, create a single example element
		// so the generated YAML template shows the expected structure.
		if val.Len() == 0 && elemType.Kind() == reflect.Struct {
			tempElem := reflect.New(elemType).Elem()
			exampleValue := m.valueToInterface(tempElem, elemType)

			return []any{exampleValue}
		}

		if val.Len() == 0 {
			return []any{}
		}

		result := make([]any, val.Len())
		for i := 0; i < val.Len(); i++ {
			elem := val.Index(i)
			result[i] = m.valueToInterface(elem, typ.Elem())
		}
		return result

	case reflect.Map:
		if val.Len() == 0 {
			return map[string]any{}
		}

		result := make(map[string]any)
		for _, key := range val.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			value := val.MapIndex(key)
			result[keyStr] = m.valueToInterface(value, typ.Elem())
		}
		return result

	case reflect.String:
		return val.String()

	case reflect.Bool:
		return val.Bool()

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if typ == reflect.TypeOf(time.Duration(0)) {
			return val.Interface().(time.Duration).String()
		}
		return val.Int()

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return val.Uint()

	case reflect.Float32, reflect.Float64:
		return val.Float()

	default:
		return val.Interface()
	}
}
