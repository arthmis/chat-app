package database

import (
	"database/sql"

	"github.com/antonlindstrom/pgstore"
)

var PgDB *sql.DB
var PgStore *pgstore.PGStore
