package tune

import (
	"flag"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// extractYAMLKeys returns YAML keys physically present in a section's YAML node.
// Uses the AST directly — does not require struct decode.
// Returns nil if the section is not found in the YAML.
func extractYAMLKeys(rootNode *yaml.Node, sectionKey string) map[string]bool {
	sectionNode := findSectionNode(rootNode, sectionKey)
	if sectionNode == nil {
		return nil
	}
	if sectionNode.Kind != yaml.MappingNode {
		return nil
	}

	keys := make(map[string]bool)
	for i := 0; i < len(sectionNode.Content); i += 2 {
		if i+1 >= len(sectionNode.Content) {
			break
		}
		keys[sectionNode.Content[i].Value] = true
	}
	return keys
}

// fieldToYAMLKey maps a struct field name to its YAML key using reflection.
// Falls back to lowercase field name if no yaml tag is present.
func fieldToYAMLKey(section Section, fieldName string) string {
	val := reflect.TypeOf(section)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return strings.ToLower(fieldName)
	}

	field, ok := val.FieldByName(fieldName)
	if !ok {
		return strings.ToLower(fieldName)
	}

	yamlTag := field.Tag.Get("yaml")
	return extractYAMLKey(yamlTag, fieldName)
}

// applyFlagsTracked returns a map of section -> fieldName for flags explicitly
// set by the user on the command line. Uses FlagSet.Visit which only visits
// flags that were actually set. Does not modify any sections.
func (m *Manager) applyFlagsTracked(sections map[string]Section) map[string]map[string]bool {
	if m.flagSet == nil || len(m.flagBindings) == 0 {
		return nil
	}

	result := make(map[string]map[string]bool)

	m.flagSet.Visit(func(f *flag.Flag) {
		binding, ok := m.flagBindings[f.Name]
		if !ok {
			return
		}
		if _, exists := sections[binding.sectionKey]; !exists {
			return
		}
		if result[binding.sectionKey] == nil {
			result[binding.sectionKey] = make(map[string]bool)
		}
		result[binding.sectionKey][binding.fieldName] = true
	})

	return result
}

// computeLockedFields merges YAML, ENV, and Flag lock sources into a single
// map[section][yamlKey]source. Higher-priority sources overwrite lower ones:
// YAML < ENV < Flag. This means if a field is set by both YAML and ENV,
// the lock source will be "env".
func (m *Manager) computeLockedFields(
	result *loadResult,
	flagSetFields map[string]map[string]bool,
) map[string]map[string]string {
	locked := make(map[string]map[string]string)

	for key, section := range result.sections {
		locks := make(map[string]string)

		// YAML locks (keys are already in yaml key format)
		for yamlKey := range result.yamlKeys[key] {
			locks[yamlKey] = "yaml"
		}

		// ENV locks (field names need mapping to yaml keys)
		for fieldName := range result.envSetFields[key] {
			yamlKey := fieldToYAMLKey(section, fieldName)
			locks[yamlKey] = "env"
		}

		// Flag locks (field names need mapping to yaml keys)
		if flagSetFields != nil {
			for fieldName := range flagSetFields[key] {
				yamlKey := fieldToYAMLKey(section, fieldName)
				locks[yamlKey] = "flag"
			}
		}

		if len(locks) > 0 {
			locked[key] = locks
		}
	}

	return locked
}
