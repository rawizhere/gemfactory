package model

import (
	"testing"
)

func TestGender(t *testing.T) {
	if FromBool(true) != GenderFemale {
		t.Error("Expected true to be GenderFemale")
	}
	if FromBool(false) != GenderMale {
		t.Error("Expected false to be GenderMale")
	}
}

func TestUniqueString(t *testing.T) {
	s := NewUniqueString("test")
	if s.String() != "test" {
		t.Errorf("Expected 'test', got %s", s.String())
	}

	var empty UniqueString
	if empty.String() != "" {
		t.Errorf("Expected empty string for zero-value, got %s", empty.String())
	}
}
