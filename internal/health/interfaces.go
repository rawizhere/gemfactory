package health

import (
	"database/sql"
)

// DatabaseInterface defines required database operations for health checks.
type DatabaseInterface interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	Ping() error
}
