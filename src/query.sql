-----------
-- USERS --
-----------

-- name: CreateUser :one
INSERT INTO user ( username, password, needs_to_reset_password, cookie ) VALUES ( ?, ?, 1, ? )
RETURNING id;

-- name: GetUserPassword :one
SELECT password FROM user WHERE id = ?;

-- name: SetUserPassword :exec
UPDATE user SET password = ? WHERE id = ?;

-- name: ResetUserPassword :exec
UPDATE user SET password = ?, needs_to_reset_password = 1, cookie = ? WHERE username = ?;

-- name: GetUserAuthDetails :one
SELECT id, password, needs_to_reset_password, enabled, cookie FROM user WHERE username = ?;

-- name: GetUsers :many
SELECT username, avatar FROM user WHERE enabled = 1 ORDER BY username;

-- name: AreThereAnyUsers :one
SELECT EXISTS( SELECT 1 FROM user LIMIT 1 );

-- name: EnableUser :exec
UPDATE user SET enabled = 1 WHERE username = ?;

-- name: DisableUser :exec
UPDATE user SET enabled = 0 WHERE username = ?;

-- name: AddAvatar :exec
INSERT OR IGNORE INTO avatar ( sha256, avatar ) VALUES ( ?, ? );

-- name: GetAvatar :one
SELECT avatar FROM avatar WHERE sha256 = ?;

-- name: SetUserAvatar :exec
UPDATE user SET avatar = ? WHERE id = ?;

-- name: DeleteUnusedAvatars :exec
DELETE FROM avatar WHERE NOT EXISTS( SELECT 1 FROM user WHERE user.avatar = avatar.sha256 );


------------
-- ASSETS --
------------

-- name: AssetExists :one
SELECT EXISTS( SELECT 1 FROM asset WHERE sha256 = ? );

-- name: CreateAsset :exec
INSERT OR IGNORE INTO asset (
	sha256, created_at, original_filename, type,
	thumbnail, thumbhash,
	description, date_taken, latitude, longitude )
VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?, ? );

-- name: AddAssetToPhoto :exec
INSERT OR IGNORE INTO photo_asset ( photo_id, asset_id ) VALUES ( ?, ? );

-- name: GetAssetMetadata :one
SELECT type, original_filename, EXISTS(
	SELECT 1 FROM photo_asset
	INNER JOIN photo ON photo.id = photo_asset.photo_id
	INNER JOIN album_photo ON album_photo.photo_id = photo.id
	INNER JOIN album ON album.id = album_photo.album_id
	-- TODO: this is failing when the photo is owned but not in an album
	WHERE photo_asset.asset_id = ? AND ( photo.owner = ? OR album.owner = ? OR album.shared )
) AS has_permission
FROM asset WHERE sha256 = ?;

-- name: GetAssetThumbnail :one
SELECT thumbnail, original_filename FROM asset WHERE sha256 = ?;

-- name: GetAssetGuestMetadata :one
SELECT type, original_filename, EXISTS(
	SELECT 1 FROM photo_asset
	INNER JOIN photo ON photo.id = photo_asset.photo_id
	INNER JOIN album_photo ON album_photo.photo_id = photo.id
	INNER JOIN album ON album.id = album_photo.album_id
	WHERE album.url_slug = ? AND ( album.readonly_secret = ? OR album.readwrite_secret = ? )
) AS has_permission
FROM asset WHERE sha256 = ?;

-- name: GetAssetGuestThumbnail :one
SELECT thumbnail, original_filename, EXISTS(
	SELECT 1 FROM photo_asset
	INNER JOIN photo ON photo.id = photo_asset.photo_id
	INNER JOIN album_photo ON album_photo.photo_id = photo.id
	INNER JOIN album ON album.id = album_photo.album_id
	WHERE album.url_slug = ? AND ( album.readonly_secret = ? OR album.readwrite_secret = ? )
) AS has_permission
FROM asset WHERE sha256 = ?;

-- name: GetAlbumAssets :many
SELECT asset.sha256 AS asset, asset.type FROM asset
INNER JOIN photo_asset ON asset.sha256 = photo_asset.asset_id
INNER JOIN photo ON photo.id = photo_asset.photo_id
INNER JOIN album_photo ON photo.id = album_photo.photo_id
INNER JOIN album ON album.id = album_photo.album_id
WHERE album.id = ? AND (
	( @include_everything OR photo.primary_asset = asset.sha256 )
	OR ( @include_raws AND asset.type = "raw" )
);


------------
-- PHOTOS --
------------

-- name: CreatePhoto :one
INSERT INTO photo ( owner, created_at, primary_asset )
VALUES( ?, ?, ? )
RETURNING id;

-- name: GetUserPhotos :many
SELECT photo.id, photo_primary_asset.sha256, photo_primary_asset.thumbhash
FROM photo
INNER JOIN photo_primary_asset ON photo.id = photo_primary_asset.photo_id
WHERE owner = ? ORDER BY photo_primary_asset.date_taken DESC;

-- name: GetAssetPhotos :many
SELECT photo.id FROM photo, photo_asset
WHERE photo_asset.asset_id = ? AND photo.owner IS ? AND photo.id = photo_asset.photo_id;

-- name: GetPhoto :one
SELECT asset.sha256, asset.type, asset.original_filename FROM photo, asset
WHERE photo.id = ? AND asset.sha256 = IFNULL( photo.primary_asset,
	( SELECT sha256 FROM asset
	INNER JOIN photo_asset ON photo_asset.asset_id = asset.sha256
	WHERE photo_asset.photo_id = photo.id AND asset.type != "raw"
	ORDER BY asset.created_at DESC LIMIT 1 ) );

-- name: GetPhotoOwner :one
SELECT owner FROM photo WHERE id = ?;

-- name: GetPhotoAssets :many
SELECT asset.sha256 AS asset, asset.type, photo.owner = ? AS owned
FROM asset
INNER JOIN photo_asset ON asset.sha256 = photo_asset.asset_id
INNER JOIN photo ON photo.id = photo_asset.photo_id
WHERE photo.id = ? AND (
	( @include_everything OR photo.primary_asset = asset.sha256 )
	OR ( @include_raws AND asset.type = "raw" )
);

-- name: GetPhotoAssetsForGuest :many
SELECT asset.sha256 AS asset, asset.type, EXISTS(
	SELECT 1 FROM album_photo
	WHERE album_photo.photo_id = ? AND album_photo.album_id = ?
) AS has_permission
FROM asset
INNER JOIN photo_asset ON asset.sha256 = photo_asset.asset_id
INNER JOIN photo ON photo.id = photo_asset.photo_id
WHERE photo.id = ? AND (
	( @include_everything OR photo.primary_asset = asset.sha256 )
	OR ( @include_raws AND asset.type = "raw" )
);


------------
-- ALBUMS --
------------

-- name: CreateAlbum :exec
INSERT INTO album (
	owner, name, url_slug,
	shared, readonly_secret, readwrite_secret,
	autoassign_start_date, autoassign_end_date, autoassign_latitude, autoassign_longitude, autoassign_radius
) VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? );

-- name: DeleteAlbum :exec
UPDATE album SET delete_at = ? WHERE url_slug = ?;

-- name: PurgeDeletedAlbums :exec
DELETE FROM album WHERE delete_at IS NOT NULL AND delete_at < ?;

-- name: RestoreDeletedAlbum :exec
UPDATE album SET delete_at = NULL WHERE url_slug = ?;

-- name: AddPhotoToAlbum :exec
INSERT OR IGNORE INTO album_photo ( album_id, photo_id ) VALUES ( ?, ? );

-- name: RemovePhotoFromAlbum :exec
DELETE FROM album_photo WHERE album_id = ? AND photo_id = ?;

-- name: GetAlbumsForUser :many
SELECT album.name, album.url_slug, album_key_asset.sha256 AS key_photo_sha256 FROM album
LEFT OUTER JOIN album_key_asset ON album.id = album_key_asset.id
WHERE ( album.shared OR album.owner = ? ) AND album.delete_at IS NULL
ORDER BY album.name;

-- name: GetAlbumByURL :one
SELECT
	album.id, album.owner, url_slug, user.username AS owner_username,
	album.name, shared, readonly_secret, readwrite_secret,
	album_key_asset.sha256 AS key_photo_sha256
FROM album
LEFT OUTER JOIN album_key_asset ON album.id = album_key_asset.id
INNER JOIN user ON album.owner = user.id
LEFT OUTER JOIN album_photo ON album_photo.album_id = album.id
LEFT OUTER JOIN photo ON album_photo.photo_id = photo.id
LEFT OUTER JOIN photo_primary_asset ON photo.id = photo_primary_asset.photo_id
WHERE url_slug = ? AND album.delete_at IS NULL;

-- name: GetAlbumDateRange :one
SELECT
	MIN( photo_primary_asset.date_taken ) AS oldest_photo,
	MAX( photo_primary_asset.date_taken ) AS newest_photo
FROM album
LEFT OUTER JOIN album_key_asset ON album.id = album_key_asset.id
INNER JOIN user ON album.owner = user.id
LEFT OUTER JOIN album_photo ON album_photo.album_id = album.id
LEFT OUTER JOIN photo ON album_photo.photo_id = photo.id
LEFT OUTER JOIN photo_primary_asset ON photo.id = photo_primary_asset.photo_id
WHERE album.id = ? AND album.delete_at IS NULL;

-- name: GetAlbumOwner :one
SELECT owner FROM album WHERE id = ?;

-- name: GetAlbumPhotos :many
SELECT photo.id, photo_primary_asset.sha256, photo_primary_asset.thumbhash
FROM photo
INNER JOIN album_photo ON album_photo.photo_id = photo.id
INNER JOIN photo_primary_asset ON photo.id = photo_primary_asset.photo_id
WHERE album_photo.album_id = ?
ORDER BY photo_primary_asset.date_taken ASC;

-- name: GetAlbumAutoassignRules :many
SELECT
	id AS album_id,
	autoassign_start_date AS start_date,
	autoassign_end_date AS end_date,
	autoassign_latitude AS latitude,
	autoassign_longitude AS longitude,
	autoassign_radius AS radius
FROM album WHERE ? BETWEEN autoassign_start_date AND autoassign_end_date;

-- name: SetAlbumSettings :exec
UPDATE album SET name = ?, url_slug = ? WHERE id = ? AND owner = ?;

-- name: SetAlbumIsShared :exec
UPDATE album SET shared = ? WHERE id = ? AND owner = ?;

-- name: IsAlbumURLInUse :one
SELECT EXISTS ( SELECT 1 FROM album WHERE url_slug = ? );
