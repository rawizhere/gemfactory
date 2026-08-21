package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"unique"
)

// UniqueString wraps unique.Handle[string] to provide interoperability with JSON and SQL.
type UniqueString struct {
	handle unique.Handle[string]
}

// NewUniqueString creates a new UniqueString with interning.
func NewUniqueString(s string) UniqueString {
	return UniqueString{handle: unique.Make(s)}
}

// String returns the underlying string value.
func (u UniqueString) String() string {
	if u.handle == (unique.Handle[string]{}) {
		return ""
	}
	return u.handle.Value()
}

// MarshalJSON implements json.Marshaler.
func (u UniqueString) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (u *UniqueString) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	u.handle = unique.Make(s)
	return nil
}

// Value implements driver.Valuer.
func (u UniqueString) Value() (driver.Value, error) {
	return u.String(), nil
}

// Scan implements sql.Scanner.
func (u *UniqueString) Scan(src any) error {
	if src == nil {
		return nil
	}
	switch v := src.(type) {
	case string:
		u.handle = unique.Make(v)
	case []byte:
		u.handle = unique.Make(string(v))
	default:
		return fmt.Errorf("unexpected type for UniqueString: %T", src)
	}
	return nil
}
