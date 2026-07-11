// Package storage owns the SQLite driver dependency without opening a database.
package storage

import _ "modernc.org/sqlite"

// DriverName is the database/sql driver name registered by modernc.org/sqlite.
const DriverName = "sqlite"
