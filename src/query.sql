-----------
-- USERS --
-----------

-- name: CreateUser :exec
INSERT INTO users ( username, password, needs_to_reset_password, cookie ) VALUES ( ?, ?, 1, ? );

-- name: ChangePassword :exec
UPDATE users SET password = ?, cookie = ? WHERE username = username;

-- name: ResetPassword :exec
UPDATE users SET password = ?, needs_to_reset_password = 1, cookie = ? WHERE username = username;

-- name: GetUserAuthDetails :one
SELECT id, password, needs_to_reset_password, cookie FROM users WHERE username = ?;

-- name: GetUsers :many
SELECT username FROM users;

-- name: AreThereAnyUsers :one
SELECT IFNULL( ( SELECT 1 FROM users LIMIT 1 ), 0 );


------------
-- ASSETS --
------------

-- name: CreateAsset :exec
INSERT OR IGNORE INTO assets ( sha256, created_at, original_filename, type, description, date_taken, latitude, longitude )
VALUES ( ?, ?, ?, ?, ?, ?, ?, ? );

-- name: AddAssetToPhoto :exec
INSERT INTO photo_assets ( photo_id, asset_id ) VALUES ( ?, ? );

-- name: GetAssetMetadata :one
SELECT type, original_filename, IFNULL( (
	SELECT 1 FROM photo_assets
	INNER JOIN photos ON photos.id = photo_assets.photo_id
	INNER JOIN album_photos ON album_photos.photo_id = photos.id
	INNER JOIN albums ON album.id = album_photos.album_id
	WHERE photo_assets.asset_id = ? AND ( photos.owner = ? OR albums.owner = ? OR albums.shared )
), 0 ) AS has_permission
FROM assets WHERE sha256 = ?;


------------
-- PHOTOS --
------------

-- name: CreatePhoto :one
INSERT INTO photos ( owner, created_at, primary_asset, thumbnail, thumbhash, date_taken, latitude, longitude )
VALUES( ?, ?, ?, ?, ?, ?, ?, ? )
RETURNING id;

-- name: GetUserPhotos :many
SELECT id, thumbhash FROM photos WHERE owner = ? ORDER BY photos.date_taken DESC;

-- name: GetAssetPhotos :many
SELECT photos.id FROM photos, photo_assets
WHERE photo_assets.asset_id = ? AND photos.owner = ? AND photos.id = photo_assets.photo_id;

-- name: GetPhoto :one
SELECT assets.sha256, assets.type, assets.original_filename FROM photos, assets
WHERE photos.id = ? AND assets.sha256 = IFNULL( photos.primary_asset,
	( SELECT sha256 FROM assets
	INNER JOIN photo_assets ON photo_assets.asset_id = assets.sha256
	WHERE photo_assets.photo_id = photos.id AND assets.type != "raw"
	ORDER BY assets.created_at DESC LIMIT 1 ) );

-- name: GetThumbnail :one
SELECT thumbnail FROM photos WHERE id = ?;


------------
-- ALBUMS --
------------

-- name: CreateAlbum :exec
INSERT INTO albums (
	owner, name, url_slug,
	shared, readonly_secret, readwrite_secret,
	autoassign_start_date, autoassign_end_date, autoassign_latitude, autoassign_longitude, autoassign_radius
) VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? );

-- name: AddPhotoToAlbum :exec
INSERT OR IGNORE INTO album_photos ( album_id, photo_id ) VALUES ( ?, ? );

-- name: GetAlbumsForUser :many
SELECT albums.name, albums.url_slug, assets.sha256 AS key_photo_sha256 FROM albums
LEFT OUTER JOIN photos ON photos.id = IFNULL( albums.key_photo, (
	SELECT photo_id FROM album_photos
	INNER JOIN photos ON photos.id = album_photos.photo_id
	WHERE album_photos.album_id = albums.id
	ORDER BY photos.date_taken DESC LIMIT 1
) )
LEFT OUTER JOIN assets ON assets.sha256 = IFNULL( photos.primary_asset, (
	SELECT asset_id FROM photo_assets
	INNER JOIN assets AS lol ON lol.sha256 = photo_assets.asset_id
	WHERE photo_assets.photo_id = photos.id AND lol.type != "raw"
	ORDER BY lol.created_at DESC LIMIT 1
) )
WHERE ( albums.shared OR albums.owner = ? )
ORDER BY albums.name;

-- name: GetAlbumByURL :one
SELECT albums.id, owner, users.username AS owner_username, albums.name, shared, readonly_secret, readwrite_secret
FROM albums
INNER JOIN users ON albums.owner = users.id
WHERE url_slug = ?;

-- name: GetAlbumOwnerByID :one
SELECT owner, shared FROM albums WHERE id = ?;

-- name: GetAlbumOwner :one
SELECT owner FROM albums WHERE id = ?;

-- name: GetAlbumPhotos :many
SELECT photos.id, photos.thumbhash
FROM photos, album_photos
WHERE album_photos.album_id = ? AND album_photos.photo_id = photos.id
ORDER BY photos.date_taken ASC;

-- name: GetAlbumAutoassignRules :many
SELECT
	id AS album_id,
	autoassign_start_date AS start_date,
	autoassign_end_date AS end_date,
	autoassign_latitude AS latitude,
	autoassign_longitude AS longitude,
	autoassign_radius AS radius
FROM albums WHERE ? BETWEEN autoassign_start_date AND autoassign_end_date;

-- name: SetAlbumIsShared :exec
UPDATE albums SET shared = ? WHERE id = ?;
