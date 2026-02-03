package telegram

import (
	"fmt"

	"github.com/mymmrac/telego"
)

// GetUserIdentifier returns a human-readable name for a user.
func GetUserIdentifier(user *telego.User) string {
	if user == nil {
		return "unknown"
	}
	if user.Username != "" {
		return user.Username
	}
	if user.FirstName != "" {
		if user.LastName != "" {
			return user.FirstName + " " + user.LastName
		}
		return user.FirstName
	}
	return fmt.Sprintf("user_%d", user.ID)
}
