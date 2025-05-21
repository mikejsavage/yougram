CREATE TABLE IF NOT EXISTS user (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL UNIQUE CHECK( username <> '' ),
	password TEXT NOT NULL,
	needs_to_reset_password INTEGER NOT NULL CHECK( needs_to_reset_password = 0 OR needs_to_reset_password = 1 ),
	cookie TEXT NOT NULL CHECK( cookie <> '' )
) STRICT;

CREATE TABLE IF NOT EXISTS asset (
	sha256 BLOB PRIMARY KEY CHECK( length( sha256 ) = 32 ),
	created_at INTEGER NOT NULL,
	original_filename TEXT NOT NULL,
	type TEXT NOT NULL CHECK( type = "jpeg" OR type = "heif" OR type = "raw" ),
	thumbnail BLOB NOT NULL,
	thumbhash BLOB NOT NULL,
	description TEXT,
	date_taken INTEGER,
	latitude REAL CHECK( latitude >= -90 AND latitude <= 90 ),
	longitude REAL CHECK( longitude >= -180 AND longitude < 180 )
) STRICT;

CREATE TABLE IF NOT EXISTS photo (
	id INTEGER PRIMARY KEY,
	owner INTEGER REFERENCES user( id ),
	created_at INTEGER NOT NULL,
	delete_at INTEGER,

	primary_asset BLOB NOT NULL REFERENCES asset( sha256 ),

	date_taken INTEGER,
	latitude REAL CHECK( latitude >= -90 AND latitude <= 90 ),
	longitude REAL CHECK( longitude >= -180 AND longitude < 180 ),

	FOREIGN KEY ( id, primary_asset ) REFERENCES photo_asset( photo_id, asset_id ) DEFERRABLE INITIALLY DEFERRED
) STRICT;

CREATE TABLE IF NOT EXISTS photo_asset (
	photo_id INTEGER NOT NULL REFERENCES photo( id ),
	asset_id BLOB NOT NULL REFERENCES asset( sha256 ),
	UNIQUE( photo_id, asset_id )
) STRICT;

CREATE VIEW IF NOT EXISTS photo_primary_asset
AS SELECT photo.id AS photo_id, asset.* FROM photo
INNER JOIN asset ON asset.sha256 = IFNULL( photo.primary_asset, (
		SELECT id FROM asset AS lol
		INNER JOIN photo_asset ON photo_asset.asset_id = lol.sha256
		WHERE photo_asset.photo_id = photo.id AND lol.type != "raw"
		ORDER BY lol.created_at DESC LIMIT 1
) );

CREATE TABLE IF NOT EXISTS album (
	id INTEGER PRIMARY KEY,
	owner INTEGER NOT NULL REFERENCES user( id ),
	name TEXT NOT NULL UNIQUE CHECK( name <> '' ),
	url_slug TEXT NOT NULL UNIQUE CHECK( url_slug <> '' ), -- maybe this can be a user defined function
	key_photo INTEGER REFERENCES photo( id ),

	shared INTEGER NOT NULL CHECK( shared = 0 OR shared = 1 ),
	readonly_secret TEXT NOT NULL,
	readwrite_secret TEXT NOT NULL,

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
	FOREIGN KEY( id, key_photo ) REFERENCES album_photo( album_id, photo_id )
) STRICT;

CREATE TABLE IF NOT EXISTS album_photo (
	album_id INTEGER NOT NULL REFERENCES album( id ),
	photo_id INTEGER NOT NULL REFERENCES photo( id ),
	UNIQUE( album_id, photo_id )
) STRICT;

CREATE VIEW IF NOT EXISTS album_key_photo
AS SELECT photo.id, asset.sha256 FROM photo
INNER JOIN asset ON asset.id = IFNULL( photo.primary_asset, (
		SELECT id FROM asset AS lol
		INNER JOIN photo_asset ON photo_asset.asset_id = lol.id
		WHERE photo_asset.photo_id = photo.id AND lol.type != "raw"
		ORDER BY lol.created_at DESC LIMIT 1
) );
