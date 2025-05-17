CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL UNIQUE CHECK( username <> '' ),
	password TEXT NOT NULL,
	needs_to_reset_password INTEGER NOT NULL CHECK( needs_to_reset_password = 0 OR needs_to_reset_password = 1 ),
	cookie TEXT NOT NULL CHECK( cookie <> '' )
) STRICT;

CREATE TABLE IF NOT EXISTS assets (
	id INTEGER PRIMARY KEY,
	sha256 BLOB CHECK( length( sha256 ) == 32 ),
	created_at INTEGER NOT NULL,
	original_filename TEXT NOT NULL,
	type TEXT NOT NULL CHECK( type = "jpeg" OR type = "heif" OR type = "raw" ),
	description TEXT,
	date_taken INTEGER,
	latitude REAL CHECK( latitude >= -90 AND latitude <= 90 ),
	longitude REAL CHECK( longitude >= -180 AND longitude < 180 )
) STRICT;

CREATE TABLE IF NOT EXISTS photos (
	id INTEGER PRIMARY KEY,
	owner INTEGER REFERENCES users( id ),
	created_at INTEGER NOT NULL,

	primary_asset INTEGER NOT NULL REFERENCES assets( id ),
	thumbnail BLOB NOT NULL,
	thumbhash BLOB NOT NULL,

	date_taken INTEGER,
	latitude REAL CHECK( latitude >= -90 AND latitude <= 90 ),
	longitude REAL CHECK( longitude >= -180 AND longitude < 180 ),

	FOREIGN KEY ( id, primary_asset ) REFERENCES photo_assets( photo_id, asset_id ) DEFERRABLE INITIALLY DEFERRED
) STRICT;

CREATE TABLE IF NOT EXISTS photo_assets (
	photo_id INTEGER NOT NULL REFERENCES photos( id ),
	asset_id INTEGER NOT NULL REFERENCES assets( id ),
	UNIQUE( photo_id, asset_id )
) STRICT;

CREATE TABLE IF NOT EXISTS albums (
	id INTEGER PRIMARY KEY,
	owner INTEGER NOT NULL REFERENCES users( id ),
	name TEXT NOT NULL UNIQUE CHECK( name <> '' ),
	url_slug TEXT NOT NULL UNIQUE CHECK( url_slug <> '' ), -- maybe this can be a user defined function
	key_photo INTEGER REFERENCES photos( id ),

	shared INTEGER NOT NULL CHECK( shared = 0 OR shared = 1 ),
	readonly_secret TEXT,
	readwrite_secret TEXT,

	autoassign_start_date INTEGER,
	autoassign_end_date INTEGER,
	autoassign_latitude REAL CHECK( IFNULL( autoassign_latitude, 0 ) BETWEEN -90 and 90 ),
	autoassign_longitude REAL CHECK( IFNULL( autoassign_longitude, 0 ) >= -180 AND IFNULL( autoassign_longitude, 0 ) < 180 ),
	autoassign_radius REAL CHECK( IFNULL( autoassign_radius, 0 ) >= 0 ),
	CHECK( 1
		AND ( autoassign_start_date IS NULL ) = ( autoassign_end_date IS NULL )
		AND ( autoassign_end_date IS NULL ) = ( autoassign_latitude IS NULL )
		AND ( autoassign_latitude IS NULL ) = ( autoassign_longitude IS NULL )
		AND ( autoassign_longitude IS NULL ) = ( autoassign_radius IS NULL )
	),
	FOREIGN KEY( id, key_photo ) REFERENCES album_photos( album_id, photo_id )
) STRICT;

CREATE TABLE IF NOT EXISTS album_photos (
	album_id INTEGER NOT NULL REFERENCES albums( id ),
	photo_id INTEGER NOT NULL REFERENCES photos( id ),
	UNIQUE( album_id, photo_id )
) STRICT;

