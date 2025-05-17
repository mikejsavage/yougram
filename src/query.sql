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


------------
-- ASSETS --
------------

-- name: CreateAsset :one
INSERT OR IGNORE INTO assets ( sha256, created_at, original_filename, type, description, date_taken, latitude, longitude )
VALUES ( ?, ?, ?, ?, ?, ?, ?, ? )
RETURNING id;

-- name: AddAssetToPhoto :exec
INSERT INTO photo_assets ( photo_id, asset_id ) VALUES ( ?, ? );

------------
-- PHOTOS --
------------

-- name: CreatePhoto :one
INSERT INTO photos ( owner, created_at, primary_asset, thumbnail, thumbhash, date_taken, latitude, longitude )
VALUES( ?, ?, ?, ?, ?, ?, ?, ? )
RETURNING id;

-- name: GetAssetPhotos :many
SELECT photos.id FROM photos, photo_assets
WHERE photo_assets.asset_id = ? AND photos.owner = ? AND photos.id = photo_assets.photo_id;

-- name: GetPhoto :one
SELECT assets.sha256, assets.type, assets.original_filename FROM photos, assets
WHERE photos.id = ? AND assets.id = IFNULL( photos.primary_asset,
	( SELECT assets.id FROM assets, photo_assets, photos
	WHERE photos.id = ? AND photo_assets.photo_id = photos.id AND photo_assets.asset_id = assets.id AND assets.type != "raw"
	ORDER BY assets.created_at LIMIT 1 ) );

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
SELECT name, url_slug, key_photo FROM albums WHERE shared OR owner = ?;

-- name: GetAlbumByURL :one
SELECT id, owner, name, shared, readonly_secret, readwrite_secret FROM albums WHERE url_slug = ?;

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
