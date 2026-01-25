package telegram

// ConfigInterface defines the contract for accessing essential bot configuration values.
type ConfigInterface interface {
	GetBotToken() string
	GetAdminUsername() string
}
