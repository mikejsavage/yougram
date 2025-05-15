-- name: CreateUser :exec
INSERT INTO users ( username, password, cookie ) VALUES ( ?, ?, ? );

-- name: CreateAlbum :exec
INSERT INTO albums ( name, url, secret, secret_valid_until ) VALUES ( ?, ?, ?, ? );

-- name: CreateAutoAssignRule :exec
INSERT INTO auto_assign_rules ( album_id, start_date, end_date, latitude, longitude, radius ) VALUES( ?, ?, ?, ?, ?, ? );
