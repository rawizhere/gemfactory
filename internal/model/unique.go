package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"unique"
)

type UniqueString struct {
	handle unique.Handle[string]
}

func NewUniqueString(s string) UniqueString {
	return UniqueString{handle: unique.Make(s)}
}

func (u UniqueString) String() string {
	if u.handle == (unique.Handle[string]{}) {
		return ""
	}
	return u.handle.Value()
}

func (u UniqueString) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

func (u *UniqueString) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	u.handle = unique.Make(s)
	return nil
}

func (u UniqueString) Value() (driver.Value, error) {
	return u.String(), nil
}

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
