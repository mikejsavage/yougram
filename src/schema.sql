-----------
-- USERS --
-----------
CREATE TABLE IF NOT EXISTS avatar (
	sha256 BLOB PRIMARY KEY CHECK( length( sha256 ) = 32 ),
	avatar BLOB NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS user (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL UNIQUE CHECK( username <> '' ),
	password TEXT NOT NULL,
	needs_to_reset_password INTEGER NOT NULL CHECK( needs_to_reset_password = 0 OR needs_to_reset_password = 1 ),
	enabled INTEGER DEFAULT 1 NOT NULL CHECK( enabled = 0 OR enabled = 1 ),
	cookie BLOB NOT NULL CHECK( length( cookie ) = 16 ),
	avatar BLOB REFERENCES avatar( sha256 )
) STRICT;

------------
-- ASSETS --
------------
CREATE TABLE IF NOT EXISTS asset (
	sha256 BLOB PRIMARY KEY CHECK( length( sha256 ) = 32 ),
	created_at INTEGER NOT NULL,
	original_filename TEXT NOT NULL,
	type TEXT NOT NULL CHECK( type = "jpg" OR type = "heic" OR type = "raw" ),
	thumbnail BLOB NOT NULL,
	thumbhash BLOB NOT NULL,
	description TEXT,
	date_taken INTEGER,
	latitude REAL CHECK( latitude >= -90 AND latitude <= 90 ),
	longitude REAL CHECK( longitude >= -180 AND longitude < 180 )
) STRICT;

CREATE INDEX IF NOT EXISTS idx_asset__created_at ON asset( created_at );
CREATE INDEX IF NOT EXISTS idx_asset__date_taken ON asset( date_taken );

------------
-- PHOTOS --
------------
CREATE TABLE IF NOT EXISTS photo (
	id INTEGER PRIMARY KEY,
	owner INTEGER REFERENCES user( id ),
	created_at INTEGER NOT NULL,
	delete_at INTEGER,
	primary_asset BLOB NOT NULL REFERENCES asset( sha256 ),
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

CREATE INDEX IF NOT EXISTS idx_photo__owner ON photo( owner );
CREATE INDEX IF NOT EXISTS idx_photo_asset__photo_id ON photo_asset( photo_id );
CREATE INDEX IF NOT EXISTS idx_photo_asset__asset_id ON photo_asset( asset_id );

------------
-- ALBUMS --
------------
CREATE TABLE IF NOT EXISTS album (
	id INTEGER PRIMARY KEY,
	owner INTEGER NOT NULL REFERENCES user( id ),
	name TEXT NOT NULL UNIQUE CHECK( name <> '' ),
	-- TODO: this forces a unique slug across all users which is really awkward
	url_slug TEXT NOT NULL UNIQUE CHECK( url_slug <> '' ),
	key_photo INTEGER REFERENCES photo( id ) ON DELETE SET NULL,

	shared INTEGER NOT NULL CHECK( shared = 0 OR shared = 1 ),
	readonly_secret TEXT NOT NULL,
	readwrite_secret TEXT NOT NULL,

	delete_at INTEGER,

	autoassign_start_date INTEGER,
	autoassign_end_date INTEGER,
	autoassign_latitude REAL CHECK( IFNULL( autoassign_latitude, 0 ) BETWEEN -90 and 90 ),
	autoassign_longitude REAL CHECK( IFNULL( autoassign_longitude, 0 ) >= -180 AND IFNULL( autoassign_longitude, 0 ) < 180 ),
	autoassign_radius REAL CHECK( IFNULL( autoassign_radius, 0 ) >= 0 ),
	CHECK( 1
		AND ( autoassign_start_date IS NULL ) = ( autoassign_end_date IS NULL ) -- require both or neither dates
		AND ( ( autoassign_end_date IS NOT NULL ) OR ( autoassign_latitude IS NULL ) ) -- pos implies date
		AND ( autoassign_latitude IS NULL ) = ( autoassign_longitude IS NULL ) -- require all or no pos fields
		AND ( autoassign_longitude IS NULL ) = ( autoassign_radius IS NULL )
	),
	FOREIGN KEY( id, key_photo ) REFERENCES album_photo( album_id, photo_id )
) STRICT;

CREATE TRIGGER IF NOT EXISTS ensure_album_secrets_are_different
AFTER INSERT ON album FOR EACH ROW
WHEN NEW.readonly_secret = NEW.readwrite_secret
BEGIN SELECT RAISE( ABORT, "readonly_secret = readwrite_secret" ); END;

CREATE TABLE IF NOT EXISTS album_photo (
	album_id INTEGER NOT NULL REFERENCES album( id ),
	photo_id INTEGER NOT NULL REFERENCES photo( id ),
	UNIQUE( album_id, photo_id )
) STRICT;

CREATE VIEW IF NOT EXISTS album_key_asset
AS SELECT album.id, photo_primary_asset.sha256 FROM album
LEFT OUTER JOIN photo_primary_asset ON photo_primary_asset.photo_id = IFNULL( album.key_photo, (
	SELECT lol.photo_id FROM album_photo
	INNER JOIN photo_primary_asset AS lol ON album_photo.photo_id = lol.photo_id
	WHERE album_photo.album_id = album.id
	ORDER BY lol.date_taken DESC LIMIT 1
) );

-- the unique constraint makes the first index pointless, not sure if we ever need the second one
-- CREATE INDEX IF NOT EXISTS idx_album_photo__album_id ON album_photo( album_id );
CREATE INDEX IF NOT EXISTS idx_album_photo__photo_id ON album_photo( photo_id );
