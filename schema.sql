CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL UNIQUE CHECK( username <> '' ),
	password TEXT NOT NULL,
	cookie TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS albums (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL UNIQUE CHECK( name <> '' ),
	url TEXT NOT NULL UNIQUE CHECK( url <> '' ),
	secret TEXT NOT NULL UNIQUE CHECK( length( secret ) >= 16 ),
	secret_valid_until INTEGER NOT NULL
) STRICT;
