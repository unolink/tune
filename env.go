package tune

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// populateEnv applies ENV variables to a section struct via reflection.
// Uses precomputed ENV names from sectionsMetadata for efficiency.
// ENV variable format: UPPER(GlobalPrefix + "_" + ConfigKey + "_" + FieldName)
// Example: Prefix="APP", Section="server", Field="Port" -> "APP_SERVER_PORT"
//
// Supported types:
//   - string
//   - int, int8, int16, int32, int64
//   - uint, uint8, uint16, uint32, uint64
//   - bool
//   - time.Duration (parsed via time.ParseDuration)
//   - pointers to all of the above
//
// Ignored:
//   - fields tagged yaml:"-"
//   - unexported fields
func (m *Manager) populateEnv(section Section) error {
	val := reflect.ValueOf(section)
	if val.Kind() != reflect.Ptr {
		return fmt.Errorf("section must be a pointer")
	}

	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("section must be a pointer to struct")
	}

	typ := val.Type()
	sectionKey := section.ConfigKey()

	metadata := m.sectionsMetadata[sectionKey]
	if metadata == nil {
		return fmt.Errorf("section %q not registered", sectionKey)
	}

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

		envName, ok := metadata.envNames[typeField.Name]
		if !ok {
			continue
		}

		envValue, exists := os.LookupEnv(envName)
		if !exists {
			continue
		}

		if err := setFieldValue(field, envValue, envName); err != nil {
			return fmt.Errorf("failed to set field %q from ENV %q: %w", typeField.Name, envName, err)
		}
	}

	return nil
}

// setFieldValue sets a struct field value from an ENV variable string.
// Handles pointers, slices/structs (via JSON), and all basic types.
// envName is used only for error messages.
func setFieldValue(field reflect.Value, envValue, envName string) error {
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setFieldValue(field.Elem(), envValue, envName)
	}

	// time.Duration is an alias for int64 but needs special handling
	// to support human-readable strings like "5s" or "100ms".
	if field.Type() == reflect.TypeOf(time.Duration(0)) {
		duration, err := time.ParseDuration(envValue)
		if err != nil {
			return fmt.Errorf("invalid duration: %w", err)
		}
		field.Set(reflect.ValueOf(duration))
		return nil
	}

	if field.Kind() == reflect.Slice || field.Kind() == reflect.Array {
		return setFieldValueFromJSON(field, envValue, envName)
	}

	if field.Kind() == reflect.Struct {
		return setFieldValueFromJSON(field, envValue, envName)
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(envValue)

	case reflect.Bool:
		val, err := strconv.ParseBool(envValue)
		if err != nil {
			return fmt.Errorf("invalid bool value: %w", err)
		}
		field.SetBool(val)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val, err := strconv.ParseInt(envValue, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid int value: %w", err)
		}
		if field.OverflowInt(val) {
			return fmt.Errorf("int value %d overflows %s", val, field.Type())
		}
		field.SetInt(val)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val, err := strconv.ParseUint(envValue, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid uint value: %w", err)
		}
		if field.OverflowUint(val) {
			return fmt.Errorf("uint value %d overflows %s", val, field.Type())
		}
		field.SetUint(val)

	case reflect.Float32, reflect.Float64:
		val, err := strconv.ParseFloat(envValue, 64)
		if err != nil {
			return fmt.Errorf("invalid float value: %w", err)
		}
		if field.OverflowFloat(val) {
			return fmt.Errorf("float value %f overflows %s", val, field.Type())
		}
		field.SetFloat(val)

	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}

	return nil
}

// setFieldValueFromJSON parses JSON from an ENV variable and sets the field value.
// Used for slices and structs.
func setFieldValueFromJSON(field reflect.Value, envValue, envName string) error {
	targetType := field.Type()
	newValue := reflect.New(targetType)

	if err := json.Unmarshal([]byte(envValue), newValue.Interface()); err != nil {
		return fmt.Errorf("failed to parse JSON from ENV %q: %w", envName, err)
	}

	field.Set(newValue.Elem())
	return nil
}
