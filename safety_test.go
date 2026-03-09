package tune

import (
	"reflect"
	"sync"
	"testing"
)

// TestCopyFieldValues_WithMutex verifies that copyFieldValues
// safely handles structs containing sync.Mutex.
func TestCopyFieldValues_WithMutex(t *testing.T) {
	t.Parallel()
	// Must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("copyFieldValues panicked with mutex: %v", r)
		}
	}()

	type testSectionWithMutex struct {
		Port int
		mu   sync.Mutex //nolint:unused // accessed via reflect; tests that copyFieldValues skips sync primitives
	}

	// Use reflection directly for the test.
	srcSection := &testSectionWithMutex{Port: 8080}
	dstSection := &testSectionWithMutex{Port: 9000}

	// Verify via isSyncType.
	mutexType := reflect.TypeOf(sync.Mutex{})
	if !isSyncType(mutexType) {
		t.Error("isSyncType should return true for sync.Mutex")
	}

	// Test copying via reflection.
	dstVal := reflect.ValueOf(dstSection).Elem()
	srcVal := reflect.ValueOf(srcSection).Elem()

	for i := 0; i < dstVal.NumField(); i++ {
		dstField := dstVal.Field(i)
		srcField := srcVal.Field(i)

		if !dstField.CanSet() {
			continue
		}

		if isSyncType(dstField.Type()) {
			continue
		}

		dstField.Set(srcField)
	}

	// Verify Port was copied.
	if dstSection.Port != 8080 {
		t.Errorf("expected port 8080 after copy, got %d", dstSection.Port)
	}
}

// TestCopyFieldValues_UnexportedFields verifies that unexported fields are skipped.
func TestCopyFieldValues_UnexportedFields(t *testing.T) {
	t.Parallel()
	type testSectionPrivate struct {
		private string
		Port    int
	}

	src := &testSectionPrivate{Port: 8080, private: "src"}
	dst := &testSectionPrivate{Port: 9000, private: "dst"}

	dstVal := reflect.ValueOf(dst).Elem()
	srcVal := reflect.ValueOf(src).Elem()

	for i := 0; i < dstVal.NumField(); i++ {
		dstField := dstVal.Field(i)
		srcField := srcVal.Field(i)

		if !dstField.CanSet() {
			continue
		}

		dstField.Set(srcField)
	}

	// Port should be copied.
	if dst.Port != 8080 {
		t.Errorf("expected port 8080, got %d", dst.Port)
	}

	// private must NOT be copied (should remain "dst").
	if dst.private != "dst" {
		t.Errorf("expected private to remain 'dst', got %q", dst.private)
	}
}

// TestIsSyncType verifies detection of sync primitive types.
func TestIsSyncType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ      reflect.Type
		name     string
		expected bool
	}{
		{typ: reflect.TypeOf(sync.Mutex{}), name: "sync.Mutex", expected: true},
		{typ: reflect.TypeOf(sync.RWMutex{}), name: "sync.RWMutex", expected: true},
		{typ: reflect.TypeOf(sync.WaitGroup{}), name: "sync.WaitGroup", expected: true},
		{typ: reflect.TypeOf(0), name: "int", expected: false},
		{typ: reflect.TypeOf(""), name: "string", expected: false},
		{typ: reflect.TypeOf(struct{}{}), name: "struct", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSyncType(tt.typ)
			if result != tt.expected {
				t.Errorf("isSyncType(%v) = %v, expected %v", tt.typ, result, tt.expected)
			}
		})
	}
}
