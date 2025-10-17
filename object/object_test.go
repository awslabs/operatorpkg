package object_test

import (
	"testing"

	"github.com/awslabs/operatorpkg/object"
)

func TestHashConsistency(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "string",
			input:    "test",
			expected: "4750482246928359241",
		},
		{
			name:     "int",
			input:    42,
			expected: "6502725087716144517",
		},
		{
			name:     "struct",
			input:    struct{ Name string }{Name: "test"},
			expected: "8856711690231820218",
		},
		{
			name:     "map",
			input:    map[string]string{"key": "value"},
			expected: "17998630672759287760",
		},
		{
			name:     "slice",
			input:    []string{"a", "b", "c"},
			expected: "16898053174053854585",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := object.Hash(tt.input)
			if result != tt.expected {
				t.Errorf("Hash() = %v, want %v", result, tt.expected)
			}
		})
	}

	// Test consistency
	t.Run("consistency", func(t *testing.T) {
		input := "consistent"
		hash1 := object.Hash(input)
		hash2 := object.Hash(input)
		if hash1 != hash2 {
			t.Errorf("Hash should be consistent: %v != %v", hash1, hash2)
		}
	})
}

type obj1 struct {
	bar map[string]string
}

func TestHashExpectedDifferences(t *testing.T) {
	testObjects := []any{
		0,
		"0",
		[]int{},
		struct{}{},
		struct {
			foo []obj1
		}{
			foo: []obj1{
				{
					bar: map[string]string{"a": "b", "c": "d"},
				},
			}},
		struct {
			foo []obj1
		}{
			foo: []obj1{
				{
					bar: map[string]string{"a": "b"},
				},
				{
					bar: map[string]string{"c": "d"},
				},
			}},
	}
	testHashes := map[string]struct{}{}
	for _, obj := range testObjects {
		hash := object.Hash(obj)
		if _, exists := testHashes[hash]; exists {
			t.Errorf("Hash collision detected: %v", hash)
		}
		testHashes[hash] = struct{}{}
	}

}
