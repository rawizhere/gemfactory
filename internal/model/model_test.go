package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGender(t *testing.T) {
	require.Equal(t, GenderFemale, FromBool(true), "expected true to be GenderFemale")
	require.Equal(t, GenderMale, FromBool(false), "expected false to be GenderMale")
}

func TestMonthNumber(t *testing.T) {
	require.Equal(t, 4, MonthNumber("April"))
	require.Equal(t, 12, MonthNumber(" december "))
	require.Zero(t, MonthNumber("Foo"), "unexpected MonthNumber results")
}
