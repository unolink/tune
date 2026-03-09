package tune

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"
)

// flagBinding holds the association between a CLI flag and a section field.
type flagBinding struct {
	value      any
	sectionKey string
	fieldName  string
	flagName   string
	assigned   bool
	isJSON     bool
}

// BindOption configures flag binding behavior.
type BindOption func(*bindOptions)

type bindOptions struct {
	flat bool
}

// WithFlatFlags enables flat flag mode (no section prefix).
// Useful for CLI commands that configure only a single section.
// Example: instead of --server.port, the flag becomes --port.
func WithFlatFlags() BindOption {
	return func(o *bindOptions) {
		o.flat = true
	}
}

// BindFlags registers flags in a flag.FlagSet based on `flag` struct tags in sections.
// Must be called BEFORE flag.Parse().
//
// Example:
//
//	manager := tune.New()
//	manager.Register(&MyConfig{})
//	manager.BindFlags(fs, tune.WithFlatFlags())
//	// Now fs.Parse() will populate temporary values
//	// Later manager.Load() will apply them to sections
func (m *Manager) BindFlags(fs *flag.FlagSet, opts ...BindOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	options := &bindOptions{}
	for _, opt := range opts {
		opt(options)
	}

	if m.flagBindings == nil {
		m.flagBindings = make(map[string]*flagBinding)
	}
	m.flagSet = fs

	for key, section := range m.sections {
		if err := m.bindSectionFlags(fs, key, section, options); err != nil {
			return fmt.Errorf("failed to bind flags for section %q: %w", key, err)
		}
	}

	return nil
}

// bindSectionFlags iterates over struct fields and registers flags for each.
func (m *Manager) bindSectionFlags(fs *flag.FlagSet, sectionKey string, section Section, opts *bindOptions) error {
	val := reflect.ValueOf(section)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		typeField := typ.Field(i)

		if !field.CanSet() {
			continue
		}

		flagTag := typeField.Tag.Get("flag")
		if flagTag == "" || flagTag == "-" {
			continue
		}

		var flagName string
		if opts.flat {
			flagName = flagTag
		} else {
			flagName = sectionKey + "." + flagTag
		}

		usage := typeField.Tag.Get("usage")
		defaultTag := typeField.Tag.Get("default")

		binding := &flagBinding{
			sectionKey: sectionKey,
			fieldName:  typeField.Name,
			flagName:   flagName,
		}

		switch field.Kind() {
		case reflect.String:
			defVal := ""
			if defaultTag != "" {
				defVal = defaultTag
			}
			binding.value = fs.String(flagName, defVal, usage)

		case reflect.Int:
			defVal := 0
			if defaultTag != "" {
				parsed, _ := strconv.Atoi(defaultTag)
				defVal = parsed
			}
			binding.value = fs.Int(flagName, defVal, usage)

		case reflect.Int64:
			if field.Type() == reflect.TypeOf(time.Duration(0)) {
				defVal := time.Duration(0)
				if defaultTag != "" {
					parsed, _ := time.ParseDuration(defaultTag)
					defVal = parsed
				}
				binding.value = fs.Duration(flagName, defVal, usage)
			} else {
				defVal := int64(0)
				if defaultTag != "" {
					parsed, _ := strconv.ParseInt(defaultTag, 10, 64)
					defVal = parsed
				}
				binding.value = fs.Int64(flagName, defVal, usage)
			}

		case reflect.Bool:
			defVal := false
			if defaultTag == "true" {
				defVal = true
			}
			binding.value = fs.Bool(flagName, defVal, usage)

		case reflect.Float64:
			defVal := 0.0
			if defaultTag != "" {
				parsed, _ := strconv.ParseFloat(defaultTag, 64)
				defVal = parsed
			}
			binding.value = fs.Float64(flagName, defVal, usage)

		case reflect.Slice, reflect.Array:
			defVal := "[]"
			if defaultTag != "" {
				defVal = defaultTag
			}
			if usage != "" {
				usage += " (JSON format, supports ${ENV_VAR} expansion)"
			} else {
				usage = "JSON format, supports ${ENV_VAR} expansion"
			}
			binding.value = fs.String(flagName, defVal, usage)
			binding.isJSON = true

		case reflect.Struct:
			// time.Duration is int64 under the hood; if we somehow reach here,
			// it indicates an internal inconsistency.
			if field.Type() == reflect.TypeOf(time.Duration(0)) {
				return fmt.Errorf("field %q: time.Duration should be int64, not struct (internal error)", typeField.Name)
			}

			defVal := "{}"
			if defaultTag != "" {
				defVal = defaultTag
			}
			if usage != "" {
				usage += " (JSON format, supports ${ENV_VAR} expansion)"
			} else {
				usage = "JSON format, supports ${ENV_VAR} expansion"
			}
			binding.value = fs.String(flagName, defVal, usage)
			binding.isJSON = true

		default:
			return fmt.Errorf("field %q: unsupported type for flag binding: %s", typeField.Name, field.Kind())
		}

		m.flagBindings[flagName] = binding
	}

	return nil
}

// applyFlags applies flag values to sections.
// Called at the end of Load(). Only applies flags explicitly set by the user.
func (m *Manager) applyFlags(sections map[string]Section) error {
	if m.flagSet == nil || len(m.flagBindings) == 0 {
		return nil
	}

	// Determine which flags were explicitly set by the user.
	// FlagSet.Visit only visits flags that were actually set.
	m.flagSet.Visit(func(f *flag.Flag) {
		if binding, ok := m.flagBindings[f.Name]; ok {
			binding.assigned = true
		}
	})

	for _, binding := range m.flagBindings {
		if !binding.assigned {
			continue
		}

		section, exists := sections[binding.sectionKey]
		if !exists {
			continue
		}

		val := reflect.ValueOf(section)
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}

		field := val.FieldByName(binding.fieldName)
		if !field.IsValid() || !field.CanSet() {
			continue
		}

		if binding.isJSON {
			rawJSON := *(binding.value.(*string))

			// Expand ENV variables directly within the JSON string,
			// enabling patterns like --flag='{"key": "${ENV_VAR}"}'.
			expandedJSON := os.ExpandEnv(rawJSON)

			newValue := reflect.New(field.Type())

			if err := json.Unmarshal([]byte(expandedJSON), newValue.Interface()); err != nil {
				return fmt.Errorf("failed to parse JSON flag --%s: %w (input: %q)",
					binding.flagName, err, expandedJSON)
			}

			field.Set(newValue.Elem())
			continue
		}

		switch v := binding.value.(type) {
		case *string:
			field.SetString(*v)
		case *int:
			field.SetInt(int64(*v))
		case *int64:
			field.SetInt(*v)
		case *time.Duration:
			field.SetInt(int64(*v))
		case *bool:
			field.SetBool(*v)
		case *float64:
			field.SetFloat(*v)
		default:
			return fmt.Errorf("unsupported flag value type: %T", v)
		}
	}

	return nil
}
