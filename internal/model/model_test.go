package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGender(t *testing.T) {
	require.Equal(t, GenderFemale, FromBool(true), "expected true to be GenderFemale")
	require.Equal(t, GenderMale, FromBool(false), "expected false to be GenderMale")
}

func TestUniqueString(t *testing.T) {
	s := NewUniqueString("test")
	require.Equal(t, "test", s.String())

	var empty UniqueString
	require.Equal(t, "", empty.String(), "expected empty string for zero-value")
}
