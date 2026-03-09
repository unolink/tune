package tune

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

// expandEnv recursively walks the struct and expands environment variables
// in string fields. Supports ${VAR} and $VAR formats.
// Applied after parsing JSON from ENV variables so that secrets
// can be substituted into complex structures.
func (m *Manager) expandEnv(section Section) error {
	val := reflect.ValueOf(section)
	if val.Kind() != reflect.Ptr {
		return fmt.Errorf("section must be a pointer")
	}

	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("section must be a pointer to struct")
	}

	return expandEnvRecursive(val)
}

// expandEnvRecursive recursively walks the value and expands variables in strings.
func expandEnvRecursive(val reflect.Value) error {
	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return nil
		}
		return expandEnvRecursive(val.Elem())

	case reflect.Struct:
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

			if err := expandEnvRecursive(field); err != nil {
				return fmt.Errorf("field %q: %w", typeField.Name, err)
			}
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			if err := expandEnvRecursive(val.Index(i)); err != nil {
				return err
			}
		}

	case reflect.Map:
		for _, key := range val.MapKeys() {
			value := val.MapIndex(key)

			// Map key expansion is not supported: replacing a key requires deleting
			// the old entry and inserting a new one, which is unsafe during iteration.

			// MapIndex returns a non-addressable value, so SetString would panic.
			// For string values we use SetMapIndex; other value kinds are skipped
			// (complex map values are rare in configuration structs).
			if value.Kind() == reflect.String {
				original := value.String()
				expanded := os.ExpandEnv(original)
				if expanded != original {
					val.SetMapIndex(key, reflect.ValueOf(expanded))
				}
			}
		}

	case reflect.String:
		original := val.String()
		expanded := os.ExpandEnv(original)
		if expanded != original {
			val.SetString(expanded)
		}
	}

	return nil
}
