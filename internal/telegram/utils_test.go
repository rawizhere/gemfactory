package telegram

import (
	"testing"

	"github.com/mymmrac/telego"
)

func TestGetUserIdentifier(t *testing.T) {
	if got := GetUserIdentifier(nil); got != "unknown" {
		t.Errorf("want 'unknown' for nil user, got %q", got)
	}

	withUsername := &telego.User{ID: 100, Username: "johndoe", FirstName: "John"}
	if got := GetUserIdentifier(withUsername); got != "johndoe" {
		t.Errorf("want 'johndoe', got %q", got)
	}

	withFullName := &telego.User{ID: 200, FirstName: "Jane", LastName: "Doe"}
	if got := GetUserIdentifier(withFullName); got != "Jane Doe" {
		t.Errorf("want 'Jane Doe', got %q", got)
	}

	withFirstNameOnly := &telego.User{ID: 300, FirstName: "Alice"}
	if got := GetUserIdentifier(withFirstNameOnly); got != "Alice" {
		t.Errorf("want 'Alice', got %q", got)
	}

	withIDOnly := &telego.User{ID: 400}
	if got := GetUserIdentifier(withIDOnly); got != "user_400" {
		t.Errorf("want 'user_400', got %q", got)
	}
}
