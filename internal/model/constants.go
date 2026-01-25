// Package model provides shared constants and enumerations.
package model

// ReleaseType distinguishes between singles, albums, and EPs.
type ReleaseType string

const (
	ReleaseTypeSingle ReleaseType = "single"
	ReleaseTypeAlbum  ReleaseType = "album"
	ReleaseTypeEP     ReleaseType = "ep"
)

// Gender classifies artist groups or individuals.
type Gender string

const (
	GenderFemale Gender = "female"
	GenderMale   Gender = "male"
	GenderMixed  Gender = "mixed"
)

func (g Gender) String() string {
	return string(g)
}

func (g Gender) IsValid() bool {
	switch g {
	case GenderFemale, GenderMale, GenderMixed:
		return true
	default:
		return false
	}
}

func (g Gender) ToBool() bool {
	return g == GenderFemale
}

func FromBool(isFemale bool) Gender {
	if isFemale {
		return GenderFemale
	}
	return GenderMale
}

// HomeworkStatus tracks the lifecycle of user assignments.
type HomeworkStatus string

const (
	HomeworkStatusActive    HomeworkStatus = "active"
	HomeworkStatusCompleted HomeworkStatus = "completed"
	HomeworkStatusExpired   HomeworkStatus = "expired"
)

// ConfigKey enumerates valid configuration settings.
type ConfigKey string

const (
	ConfigKeySpotifyClientID     ConfigKey = "spotify_client_id"
	ConfigKeySpotifyClientSecret ConfigKey = "spotify_client_secret"
	ConfigKeyPlaylistURL         ConfigKey = "playlist_url"
	ConfigKeyBotToken            ConfigKey = "bot_token"
	ConfigKeyAdminUsername       ConfigKey = "admin_username"
	ConfigKeyTimezone            ConfigKey = "timezone"
	ConfigKeyHealthPort          ConfigKey = "health_port"
	ConfigKeyLogLevel            ConfigKey = "log_level"
)
