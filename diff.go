package tune

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Diff compares two configuration sections and returns a list of changes.
// Each change is formatted as "Key: OldVal -> NewVal".
// Secret fields (secret:"true") are reported as "Key: changed".
func Diff(oldSection, newSection Section) []string {
	if oldSection == nil || newSection == nil {
		return nil
	}

	oldType := reflect.TypeOf(oldSection).Elem()
	newType := reflect.TypeOf(newSection).Elem()
	if oldType != newType {
		return []string{fmt.Sprintf("Section type mismatch: %s != %s", oldType, newType)}
	}

	var changes []string
	oldVal := reflect.ValueOf(oldSection).Elem()
	newVal := reflect.ValueOf(newSection).Elem()

	diffRecursive(oldVal, newVal, oldType, "", &changes)
	return changes
}

// diffRecursive recursively compares values and collects changes.
func diffRecursive(oldVal, newVal reflect.Value, typ reflect.Type, prefix string, changes *[]string) {
	switch oldVal.Kind() {
	case reflect.Ptr:
		if oldVal.IsNil() && newVal.IsNil() {
			return
		}
		if oldVal.IsNil() || newVal.IsNil() {
			*changes = append(*changes, fmt.Sprintf("%s: %v -> %v", prefix, getValueString(oldVal), getValueString(newVal)))
			return
		}
		diffRecursive(oldVal.Elem(), newVal.Elem(), typ.Elem(), prefix, changes)

	case reflect.Struct:
		if typ == reflect.TypeOf(time.Duration(0)) {
			oldDur := oldVal.Interface().(time.Duration)
			newDur := newVal.Interface().(time.Duration)
			if oldDur != newDur {
				*changes = append(*changes, fmt.Sprintf("%s: %s -> %s", prefix, oldDur.String(), newDur.String()))
			}
			return
		}

		for i := 0; i < oldVal.NumField(); i++ {
			oldField := oldVal.Field(i)
			newField := newVal.Field(i)
			typeField := typ.Field(i)

			if !oldField.CanSet() {
				continue
			}

			yamlTag := typeField.Tag.Get("yaml")
			if strings.HasPrefix(yamlTag, "-") {
				continue
			}

			yamlKey := extractYAMLKey(yamlTag, typeField.Name)
			fieldPrefix := yamlKey
			if prefix != "" {
				fieldPrefix = prefix + "." + yamlKey
			}

			secretTag := typeField.Tag.Get("secret")
			isSecret := secretTag == "true"

			// Nested structs: recurse into fields only, skip DeepEqual to avoid duplicates
			if oldField.Kind() == reflect.Struct && typeField.Type != reflect.TypeOf(time.Duration(0)) {
				diffRecursive(oldField, newField, typeField.Type, fieldPrefix, changes)
				continue
			}

			if !reflect.DeepEqual(oldField.Interface(), newField.Interface()) {
				if isSecret {
					*changes = append(*changes, fmt.Sprintf("%s: changed", fieldPrefix))
				} else {
					oldStr := getValueString(oldField)
					newStr := getValueString(newField)
					*changes = append(*changes, fmt.Sprintf("%s: %s -> %s", fieldPrefix, oldStr, newStr))
				}
			}
		}

	case reflect.Slice, reflect.Array:
		oldLen := oldVal.Len()
		newLen := newVal.Len()
		if oldLen != newLen {
			*changes = append(*changes, fmt.Sprintf("%s: length %d -> %d", prefix, oldLen, newLen))
		}

		minLen := oldLen
		if newLen < minLen {
			minLen = newLen
		}

		for i := 0; i < minLen; i++ {
			oldElem := oldVal.Index(i)
			newElem := newVal.Index(i)
			elemPrefix := fmt.Sprintf("%s[%d]", prefix, i)
			diffRecursive(oldElem, newElem, typ.Elem(), elemPrefix, changes)
		}

	case reflect.Map:
		oldKeys := oldVal.MapKeys()
		newKeys := newVal.MapKeys()

		for _, oldKey := range oldKeys {
			found := false
			for _, newKey := range newKeys {
				if reflect.DeepEqual(oldKey.Interface(), newKey.Interface()) {
					found = true
					break
				}
			}
			if !found {
				keyStr := fmt.Sprintf("%v", oldKey.Interface())
				*changes = append(*changes, fmt.Sprintf("%s[%s]: removed", prefix, keyStr))
			}
		}

		for _, newKey := range newKeys {
			keyStr := fmt.Sprintf("%v", newKey.Interface())
			// Fetch both values once since MapIndex is expensive on large maps.
			newValue := newVal.MapIndex(newKey)
			oldValue := oldVal.MapIndex(newKey)

			if !oldValue.IsValid() {
				*changes = append(*changes, fmt.Sprintf("%s[%s]: added -> %s", prefix, keyStr, getValueString(newValue)))
			} else {
				keyPrefix := fmt.Sprintf("%s[%s]", prefix, keyStr)
				diffRecursive(oldValue, newValue, typ.Elem(), keyPrefix, changes)
			}
		}

	default:
		if !reflect.DeepEqual(oldVal.Interface(), newVal.Interface()) {
			oldStr := getValueString(oldVal)
			newStr := getValueString(newVal)
			*changes = append(*changes, fmt.Sprintf("%s: %s -> %s", prefix, oldStr, newStr))
		}
	}
}

// getValueString returns a human-readable string representation of a reflect.Value for diff output.
func getValueString(val reflect.Value) string {
	if !val.IsValid() {
		return "<nil>"
	}

	switch val.Kind() {
	case reflect.Ptr:
		if val.IsNil() {
			return "<nil>"
		}
		return getValueString(val.Elem())

	case reflect.String:
		return fmt.Sprintf("%q", val.String())

	case reflect.Bool:
		return fmt.Sprintf("%v", val.Bool())

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val.Type() == reflect.TypeOf(time.Duration(0)) {
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
		if val.Type() == reflect.TypeOf(time.Duration(0)) {
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
