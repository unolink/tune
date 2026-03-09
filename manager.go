package tune

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Section interface must be implemented by package configuration structures.
// Enables the config engine to work with any structure through a unified interface.
type Section interface {
	// ConfigKey returns the section key in YAML file (e.g., "server", "logger").
	ConfigKey() string

	// SetDefaults sets default values before loading from files.
	// Called before YAML parsing and ENV variable application.
	SetDefaults()

	// Validate checks configuration correctness after loading.
	Validate() error

	// OnUpdate is called during hot-reload.
	// Allows the package to update internal state when config changes.
	OnUpdate()
}

// Logger interface for logging configuration changes.
// If not set, changes are printed to stdout.
type Logger interface {
	InfoContext(ctx context.Context, msg string, args ...any)
}

// sectionMetadata contains pre-computed section metadata for optimization
type sectionMetadata struct {
	envNames map[string]string // fieldName -> ENV_NAME (pre-computed)
}

// Manager manages loading and updating configuration for all registered sections.
// Uses Registry pattern to decouple configuration logic from business packages.
type Manager struct {
	logger           Logger
	sections         map[string]Section
	sectionsMetadata map[string]*sectionMetadata
	fileModTimes     map[string]time.Time
	metadataCache    map[string][]FieldInfo
	lockedFields     map[string]map[string]string // section -> yamlKey -> source ("yaml"/"env"/"flag")
	stopWatcher      chan struct{}
	flagSet          *flag.FlagSet
	flagBindings     map[string]*flagBinding
	globalPrefix     string
	configPath       string
	watcherWg        sync.WaitGroup
	mu               sync.RWMutex
	watching         bool
}

// Option function for configuring Manager using Functional Options pattern
type Option func(*Manager)

// WithPath sets configuration path (file or directory).
// If not set, only ENV variables are used.
//
// Examples:
//   - WithPath("/etc/myapp/config.yml") - single YAML file
//   - WithPath("/etc/myapp/config.d") - directory with all YAML files
func WithPath(path string) Option {
	return func(m *Manager) {
		m.configPath = path
	}
}

// WithEnvPrefix sets global prefix for ENV variables.
// If not set, prefix is empty.
//
// Example:
//   - WithEnvPrefix("MYAPP") -> ENV variables: MYAPP_SERVER_PORT, MYAPP_SERVER_HOST, etc.
func WithEnvPrefix(prefix string) Option {
	return func(m *Manager) {
		m.globalPrefix = prefix
	}
}

// New creates a new Manager instance with optional configuration.
//
// Usage:
//
//	// ENV variables only (no prefix)
//	m := tune.New()
//
//	// ENV with prefix
//	m := tune.New(tune.WithEnvPrefix("MYAPP"))
//
//	// File + ENV with prefix
//	m := tune.New(
//	    tune.WithPath("/etc/myapp/config.yml"),
//	    tune.WithEnvPrefix("MYAPP"),
//	)
//
//	// Directory + ENV
//	m := tune.New(
//	    tune.WithPath("/etc/myapp/config.d"),
//	    tune.WithEnvPrefix("MYAPP"),
//	)
func New(opts ...Option) *Manager {
	m := &Manager{
		configPath:       "",
		globalPrefix:     "",
		sections:         make(map[string]Section),
		sectionsMetadata: make(map[string]*sectionMetadata),
		fileModTimes:     make(map[string]time.Time),
		metadataCache:    make(map[string][]FieldInfo),
		stopWatcher:      make(chan struct{}),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// ApplyOptions applies options to an already created Manager.
// Useful when Manager is created early (e.g., for BindFlags),
// but the config path becomes known later (from --config flag).
//
// Example:
//
//	manager := tune.New(tune.WithEnvPrefix("MYAPP"))
//	manager.BindFlags(fs)
//	// ... fs.Parse() ...
//	configPath := fs.Lookup("config").Value.String()
//	manager.ApplyOptions(tune.WithPath(configPath))
//	manager.Load()
func (m *Manager) ApplyOptions(opts ...Option) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, opt := range opts {
		opt(m)
	}
}

// MustRegister calls [Manager.Register] and panics on error.
func (m *Manager) MustRegister(s Section) {
	if err := m.Register(s); err != nil {
		panic(err)
	}
}

// Register registers a configuration section with the manager.
// If a section with the same ConfigKey() is already registered, it will be overwritten.
// Pre-computes ENV variable names at registration time to optimize Load().
func (m *Manager) Register(s Section) error {
	if s == nil {
		return fmt.Errorf("config: cannot register nil section")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := s.ConfigKey()
	if key == "" {
		return fmt.Errorf("config: ConfigKey() returned empty string")
	}

	m.sections[key] = s

	m.sectionsMetadata[key] = m.buildSectionMetadata(s)
	return nil
}

// envPrefix builds the ENV variable prefix for a section, handling empty globalPrefix correctly.
func envPrefix(globalPrefix, sectionKey string) string {
	if globalPrefix != "" {
		return strings.ToUpper(globalPrefix) + "_" + strings.ToUpper(sectionKey) + "_"
	}
	return strings.ToUpper(sectionKey) + "_"
}

// buildSectionMetadata builds section metadata with pre-computed ENV variable names.
func (m *Manager) buildSectionMetadata(section Section) *sectionMetadata {
	val := reflect.ValueOf(section)
	if val.Kind() != reflect.Ptr {
		return &sectionMetadata{envNames: make(map[string]string)}
	}

	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return &sectionMetadata{envNames: make(map[string]string)}
	}

	typ := val.Type()
	sectionKey := section.ConfigKey()
	prefix := envPrefix(m.globalPrefix, sectionKey)

	envNames := make(map[string]string)
	for i := 0; i < val.NumField(); i++ {
		typeField := typ.Field(i)

		if !typeField.IsExported() {
			continue
		}

		yamlTag := typeField.Tag.Get("yaml")
		if strings.HasPrefix(yamlTag, "-") {
			continue
		}

		envName := prefix + strings.ToUpper(typeField.Name)
		envNames[typeField.Name] = envName
	}

	return &sectionMetadata{
		envNames: envNames,
	}
}

// Load performs the full configuration loading cycle in strict order:
// Defaults -> YAML -> ENV -> Flags -> Validate.
// Pure read-only: reads files and environment variables, does not access the database.
//
// Thread-safe: uses write lock to protect against concurrent access.
func (m *Manager) Load() error {
	result, err := m.loadSectionsInternal()
	if err != nil {
		return err
	}

	flagSetFields := m.applyFlagsTracked(result.sections)
	if err := m.applyFlags(result.sections); err != nil {
		return fmt.Errorf("failed to apply flags: %w", err)
	}

	for key, section := range result.sections {
		if err := section.Validate(); err != nil {
			return fmt.Errorf("validation failed for section %q: %w", key, err)
		}
	}

	// Compute locked fields from YAML + ENV + Flag sources
	lockedFields := m.computeLockedFields(result, flagSetFields)

	m.mu.Lock()

	updatedSections := make([]Section, 0, len(result.sections))

	for key, newSection := range result.sections {
		if oldSection, exists := m.sections[key]; exists {
			copyFieldValues(oldSection, newSection)
			updatedSections = append(updatedSections, oldSection)
		} else {
			m.sections[key] = newSection
			updatedSections = append(updatedSections, newSection)
		}
	}

	m.fileModTimes = result.fileModTimes
	m.lockedFields = lockedFields
	m.mu.Unlock()

	// OnUpdate is called outside the lock to initialize unexported fields (e.g., compiledRegex)
	for _, section := range updatedSections {
		func(s Section) {
			defer func() {
				if r := recover(); r != nil {
					m.logChange(context.TODO(), "OnUpdate panic during initial load",
						"section", s.ConfigKey(), "panic", r)
				}
			}()
			s.OnUpdate()
		}(section)
	}

	return nil
}

// loadResult contains output from loadSectionsInternal for the caller to process.
type loadResult struct {
	sections     map[string]Section
	fileModTimes map[string]time.Time
	yamlKeys     map[string]map[string]bool // section -> set of YAML keys present in file
	envSetFields map[string]map[string]bool // section -> set of field names set from ENV
}

// loadSectionsInternal loads configuration without modifying current sections.
// Returns temporary section instances with loaded configuration,
// plus metadata about YAML keys and ENV fields for computing locked fields.
// Safe to call without holding Manager.mu (Copy-on-Write pattern).
func (m *Manager) loadSectionsInternal() (*loadResult, error) {
	configPath := m.configPath

	rawConfig, fileModTimes, err := loadYAMLConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load YAML config: %w", err)
	}

	m.mu.RLock()
	newSections := make(map[string]Section, len(m.sections))
	for key, section := range m.sections {
		sectionType := reflect.TypeOf(section).Elem()
		newSection := reflect.New(sectionType).Interface().(Section)
		newSections[key] = newSection
	}
	m.mu.RUnlock()

	envSetFields := make(map[string]map[string]bool, len(newSections))
	for key, section := range newSections {
		section.SetDefaults()

		if err := decodeSection(rawConfig, key, section); err != nil {
			return nil, fmt.Errorf("failed to decode section %q: %w", key, err)
		}

		setFields, envErr := m.populateAndExpandEnvTracked(section)
		if envErr != nil {
			return nil, fmt.Errorf("failed to apply and expand ENV for section %q: %w", key, envErr)
		}
		if len(setFields) > 0 {
			envSetFields[key] = setFields
		}

		// Validation is deferred to Load() after applyFlags()
		// Priority order: Defaults -> YAML -> ENV -> FLAGS -> Validate
	}

	yamlKeys := make(map[string]map[string]bool, len(newSections))
	for key := range newSections {
		keys := extractYAMLKeys(rawConfig, key)
		if len(keys) > 0 {
			yamlKeys[key] = keys
		}
	}

	return &loadResult{
		sections:     newSections,
		fileModTimes: fileModTimes,
		yamlKeys:     yamlKeys,
		envSetFields: envSetFields,
	}, nil
}

// Get returns the loaded configuration section by key.
// Returns nil if the section is not registered.
// Thread-safe: uses read lock for concurrent access.
//
// Example:
//
//	serverCfg := manager.Get("server").(*ServerConfig)
func (m *Manager) Get(key string) Section {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sections[key]
}

// LockedFields returns field names locked by external sources (YAML, ENV, flags)
// for the given section. Returns map[yamlKey]source where source is "yaml", "env", or "flag".
// Returns nil if section not found or no locked fields.
func (m *Manager) LockedFields(sectionKey string) map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lockedFields == nil {
		return nil
	}
	locks, ok := m.lockedFields[sectionKey]
	if !ok {
		return nil
	}
	cp := make(map[string]string, len(locks))
	for k, v := range locks {
		cp[k] = v
	}
	return cp
}

// SetLogger sets the logger for reporting configuration changes during hot-reload.
// If not set, changes are printed to stdout via fmt.Printf.
func (m *Manager) SetLogger(logger Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

// Watch starts a background file watcher that monitors configuration changes.
// Checks file ModTime at the given interval.
// On change, automatically reloads configuration and calls OnUpdate().
// Logs all configuration changes via SetLogger or stdout.
//
// Uses Copy-on-Write pattern: IO operations (reading files, parsing YAML)
// run without holding Manager lock. The lock is only held for the atomic pointer swap.
// OnUpdate() is called outside the lock, so config readers are never blocked.
//
// Can only be called once. Use StopWatch() to stop.
func (m *Manager) Watch(interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("watch interval must be positive")
	}

	m.mu.Lock()
	if m.watching {
		m.mu.Unlock()
		return fmt.Errorf("watcher is already running")
	}
	if m.stopWatcher == nil {
		m.mu.Unlock()
		return fmt.Errorf("watcher is not initialized or already stopped")
	}
	m.watching = true
	// Copy channel to local scope to avoid races with StopWatch()
	stopCh := m.stopWatcher
	m.mu.Unlock()

	m.watcherWg.Add(1)
	go func() {
		defer m.watcherWg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return

			case <-ticker.C:
				if !m.checkIfModified() {
					continue
				}

				m.mu.RLock()
				oldSections := m.copySections()
				m.mu.RUnlock()

				result, err := m.loadSectionsInternal()
				if err != nil {
					m.logChange(context.TODO(), "Config reload failed", "err", err)
					continue
				}

				lockedFields := m.computeLockedFields(result, nil)

				// Copy values into existing sections to preserve direct reference validity
				m.mu.Lock()
				for key, newSection := range result.sections {
					if oldSection, exists := m.sections[key]; exists {
						copyFieldValues(oldSection, newSection)
					} else {
						m.sections[key] = newSection
					}
				}
				m.fileModTimes = result.fileModTimes
				m.lockedFields = lockedFields
				m.mu.Unlock()

				changedSectionKeys := m.getChangedSections(oldSections, result.sections)

				m.logChanges(oldSections, result.sections)

				m.mu.RLock()
				changedSections := make([]Section, 0, len(changedSectionKeys))
				for key := range changedSectionKeys {
					if section, exists := m.sections[key]; exists {
						changedSections = append(changedSections, section)
					}
				}
				m.mu.RUnlock()

				for _, section := range changedSections {
					func(s Section) {
						defer func() {
							if r := recover(); r != nil {
								m.logChange(context.TODO(), "OnUpdate panic recovered", "section", s.ConfigKey(), "panic", r)
							}
						}()
						s.OnUpdate()
					}(section)
				}
			}
		}
	}()

	return nil
}

// copySections creates a shallow copy of all sections for comparison.
func (m *Manager) copySections() map[string]Section {
	copies := make(map[string]Section)
	for key, section := range m.sections {
		sectionType := reflect.TypeOf(section).Elem()
		newSection := reflect.New(sectionType).Interface().(Section)

		copyFieldValues(newSection, section)

		copies[key] = newSection
	}
	return copies
}

// copyFieldValues copies field values from src to dst.
// Used to update existing sections without replacing pointers.
// Safely skips unexported fields and sync.* types to prevent panics.
func copyFieldValues(dst, src Section) {
	dstVal := reflect.ValueOf(dst).Elem()
	srcVal := reflect.ValueOf(src).Elem()

	for i := 0; i < dstVal.NumField(); i++ {
		dstField := dstVal.Field(i)
		srcField := srcVal.Field(i)

		if !dstField.CanSet() {
			continue
		}

		// sync.Mutex must not be copied by value
		if isSyncType(dstField.Type()) {
			continue
		}

		dstField.Set(srcField)
	}
}

// isSyncType reports whether t is a sync primitive type.
func isSyncType(t reflect.Type) bool {
	path := t.PkgPath()
	return path == "sync" || path == "sync/atomic"
}

// getChangedSections returns the set of section keys that have changed.
// Uses Diff for field-level comparison.
func (m *Manager) getChangedSections(oldSections, newSections map[string]Section) map[string]bool {
	changed := make(map[string]bool)

	for key, newSection := range newSections {
		oldSection, exists := oldSections[key]
		if !exists {
			changed[key] = true
			continue
		}

		changes := Diff(oldSection, newSection)
		if len(changes) > 0 {
			changed[key] = true
		}
	}

	for key := range oldSections {
		if _, exists := newSections[key]; !exists {
			changed[key] = true
		}
	}

	return changed
}

// logChanges compares old and new sections and logs the differences.
func (m *Manager) logChanges(oldSections, newSections map[string]Section) {
	ctx := context.TODO()
	for key, newSection := range newSections {
		oldSection, exists := oldSections[key]
		if !exists {
			m.logChange(ctx, "Config section added", "key", key)
			continue
		}

		changes := Diff(oldSection, newSection)
		if len(changes) > 0 {
			args := make([]any, 0, len(changes)*2)
			for i, change := range changes {
				args = append(args, "change"+strconv.Itoa(i), change)
			}
			m.logChange(ctx, "Config section changed:", args...)
		}
	}

	for key := range oldSections {
		if _, exists := newSections[key]; !exists {
			m.logChange(ctx, "Config section: removed", "key", key)
		}
	}
}

// logChange logs a message via the configured logger or stdout.
func (m *Manager) logChange(ctx context.Context, message string, args ...any) {
	m.mu.RLock()
	logger := m.logger
	m.mu.RUnlock()

	if logger != nil {
		logger.InfoContext(ctx, message, args...)
	} else {
		fmt.Println(append([]any{message}, args...)...)
	}
}

// StopWatch stops the background watcher.
// Blocks until the current check iteration completes.
func (m *Manager) StopWatch() {
	m.mu.Lock()
	if m.stopWatcher != nil {
		close(m.stopWatcher)
		m.stopWatcher = nil
	}
	m.watching = false
	m.mu.Unlock()

	m.watcherWg.Wait()
}

// checkIfModified reports whether configuration files have been modified.
// Compares ModTime of each file against saved values.
// Works correctly on all filesystems (Docker volumes, NFS, etc.).
//
// Supports three modes:
//  1. Empty path - always false (no files to watch)
//  2. Single file - checks file ModTime
//  3. Directory - checks all YAML files in directory
func (m *Manager) checkIfModified() bool {
	m.mu.RLock()
	savedModTimes := m.fileModTimes
	configPath := m.configPath
	m.mu.RUnlock()

	if configPath == "" {
		return false
	}

	info, err := os.Stat(configPath)
	if err != nil {
		return false
	}

	if !info.IsDir() {
		return checkSingleFileModified(configPath, info, savedModTimes)
	}

	return checkDirectoryModified(configPath, savedModTimes)
}

// checkSingleFileModified reports whether a single config file has been modified.
func checkSingleFileModified(filePath string, info os.FileInfo, savedModTimes map[string]time.Time) bool {
	fileName := filepath.Base(filePath)

	if savedTime, exists := savedModTimes[fileName]; !exists {
		return true
	} else if info.ModTime().After(savedTime) {
		return true
	}

	return false
}

// checkDirectoryModified reports whether any config files in the directory have been modified.
func checkDirectoryModified(dirPath string, savedModTimes map[string]time.Time) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if savedTime, exists := savedModTimes[name]; !exists {
			return true
		} else if info.ModTime().After(savedTime) {
			return true
		}
	}

	currentFiles := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			name := entry.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".yml" || ext == ".yaml" {
				currentFiles[name] = true
			}
		}
	}

	for savedFile := range savedModTimes {
		if !currentFiles[savedFile] {
			return true
		}
	}

	return false
}

// populateAndExpandEnvTracked works like populateAndExpandEnv but also returns
// a set of field names that were actually set from ENV variables.
// Used by loadSectionsInternal to compute locked fields.
func (m *Manager) populateAndExpandEnvTracked(section Section) (map[string]bool, error) {
	val := reflect.ValueOf(section)
	if val.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("section must be a pointer")
	}

	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("section must be a pointer to struct")
	}

	typ := val.Type()
	sectionKey := section.ConfigKey()

	metadata := m.sectionsMetadata[sectionKey]
	if metadata == nil {
		return nil, fmt.Errorf("section %q not registered", sectionKey)
	}

	setFields := make(map[string]bool)

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
			return nil, fmt.Errorf("failed to set field %q from ENV %q: %w", typeField.Name, envName, err)
		}
		setFields[typeField.Name] = true
	}

	if err := expandEnvRecursive(val); err != nil {
		return nil, err
	}
	return setFields, nil
}
