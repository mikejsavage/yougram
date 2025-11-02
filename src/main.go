package main

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	"image/draw"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mikegram/moondream"
	"mikegram/sqlc"
	"mikegram/stb"

	/*
	 * this library compiles avif c libs to wasm and runs them in an
	 * interpreter. obviously this is insane and adds like 5MB of bin size
	 * because the compiler can't strip dead code but avif libraries are all
	 * linux shitware and it's the only reasonable way to use them
	 */
	"github.com/gen2brain/avif"

	"github.com/adrium/goheif"
	"github.com/galdor/go-thumbhash"
	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/meta"
	"github.com/fsnotify/fsnotify"

	"golang.org/x/image/webp"

	_ "github.com/mattn/go-sqlite3"
	// sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

const megabyte = 1000 * 1000
const green_checkmark = "&#x2705;"

var db *sql.DB
var queries *sqlc.Queries

//go:embed schema.sql
var db_schema string

//go:embed vendor_js/alpine-3.14.9.js
var alpinejs string
//go:embed vendor_js/htmx-2.0.4.js
var htmxjs string
//go:embed vendor_js/thumbhash.js
var thumbhashjs string

var checksum string

var guest_url string

func sel[ T any ]( p bool, t T, f T ) T {
	if p {
		return t
	}
	return f
}

func mustImpl( err error ) {
	if err != nil {
		pc, filename, line, _ := runtime.Caller( 2 )
		f := runtime.FuncForPC( pc )
		log.Fatalf( "%s(%s:%d): %v", f.Name(), filename, line, err )
	}
}

func must( err error ) {
	mustImpl( err )
}

func must1[ T1 any ]( v1 T1, err error ) T1 {
	mustImpl( err )
	return v1
}

type WrappedError struct {
	Err error
	Function string
	Filename string
	Line int
}

func ( w WrappedError ) Error() string {
	return fmt.Sprintf( "%s(%s:%d): %v", w.Function, w.Filename, w.Line, w.Err )
}

func wrapError( err error ) WrappedError {
	pc, filename, line, _ := runtime.Caller( 2 )
	f := runtime.FuncForPC( pc )
	return WrappedError { err, f.Name(), filename, line }
}

func try( err error ) {
	if err != nil {
		panic( wrapError( err ) )
	}
}

func try1[ T1 any ]( v1 T1, err error ) T1 {
	try( err )
	return v1
}

func try2[ T1 any, T2 any ]( v1 T1, v2 T2, err error ) ( T1, T2 ) {
	try( err )
	return v1, v2
}

func exec( ctx context.Context, query string, args ...interface{} ) {
	_, err := db.ExecContext( ctx, query, args... )
	if err != nil {
		log.Fatalf( "%+v: %s", err, query )
	}
}

func queryOne[ T1 any ]( ctx context.Context, query string, args ...interface{} ) T1 {
	row := db.QueryRowContext( ctx, query, args... )
	var res T1
	must( row.Scan( &res ) )
	return res
}

func just[ T any ]( x T ) sql.Null[ T ] {
	return sql.Null[ T ] { x, true }
}

func justI64( x int64 ) sql.NullInt64 {
	return sql.NullInt64 { x, true }
}

func queryOptional[ T any ]( row T, err error ) sql.Null[ T ] {
	if err != nil {
		if errors.Is( err, sql.ErrNoRows ) {
			return sql.Null[ T ] { }
		}
		panic( err )
	}
	return just( row )
}

func initDB( memory_db bool ) {
	ctx := context.Background()

	const application_id = -133015034
	const schema_version = 1

	{
		id := queryOne[ int32 ]( ctx, "PRAGMA application_id" )
		version := queryOne[ int32 ]( ctx, "PRAGMA user_version" )

		if id != 0 && version != 0 {
			if id != application_id {
				log.Fatal( "This doesn't look like a mikegram DB" )
			}

			if version < schema_version {
				log.Fatal( "You are using an older mikegram than the DB" )
			}
			if version > schema_version {
				log.Fatal( "Guess we gotta migrate lol" )
			}
		}
	}

	exec( ctx, fmt.Sprintf( "PRAGMA application_id = %d", application_id ) )
	exec( ctx, fmt.Sprintf( "PRAGMA user_version = %d", schema_version ) )

	exec( ctx, "PRAGMA foreign_keys = ON" )
	exec( ctx, "PRAGMA journal_mode = WAL" )
	exec( ctx, "PRAGMA synchronous = NORMAL" )
	exec( ctx, "PRAGMA integrity_check" )
	exec( ctx, "PRAGMA foreign_key_check" )

	exec( ctx, db_schema )

	if !memory_db {
		return
	}

	var secret [16]byte
	mike := must1( queries.CreateUser( ctx, sqlc.CreateUserParams {
		Username: unicodeNormalize( "mike" ),
		Password: hashPassword( "gg" ),
		Cookie: secret[:],
	} ) )

	must1( queries.SetUserPasswordIfMustReset( ctx, sqlc.SetUserPasswordIfMustResetParams {
		ID: mike,
		Password: hashPassword( "gg" ),
	} ) )

	_ = must1( queries.CreateUser( ctx, sqlc.CreateUserParams {
		Username: unicodeNormalize( "mum" ),
		Password: hashPassword( "gg" ),
		Cookie: secret[:],
	} ) )

	_ = must1( queries.CreateUser( ctx, sqlc.CreateUserParams {
		Username: unicodeNormalize( "dad" ),
		Password: hashPassword( "gg" ),
		Cookie: secret[:],
	} ) )

	must( queries.CreateAlbum( ctx, sqlc.CreateAlbumParams {
		Owner: mike,
		Name: "France 2024",
		UrlSlug: "france-2024",
		Shared: 1,
		ReadonlySecret: "aaaaaaaa",
		ReadwriteSecret: "bbbbbbbb",
	} ) )
	must( queries.CreateAlbum( ctx, sqlc.CreateAlbumParams {
		Owner: mike,
		Name: "Helsinki 2024",
		UrlSlug: "helsinki-2024",
		Shared: 0,
		ReadonlySecret: "aaaaaaaa",
		ReadwriteSecret: "bbbbbbbb",
		AutoassignStartDate: justI64( time.Date( 2024, time.January, 1, 0, 0, 0, 0, time.UTC ).Unix() ),
		AutoassignEndDate: justI64( time.Date( 2024, time.December, 31, 0, 0, 0, 0, time.UTC ).Unix() ),
		AutoassignLatitude: sql.NullFloat64 { 60.1699, true },
		AutoassignLongitude: sql.NullFloat64 { 24.9384, true },
		AutoassignRadius: sql.NullFloat64 { 50, true },
	} ) )

	must( addFileToAlbum( ctx, mike, "DSCN0025.jpg", 2 ) )
	must( addFileToAlbum( ctx, mike, "DSCF2994.jpeg", 1 ) )
	must( addFileToAlbum( ctx, mike, "hato.profile0.8bpc.yuv420.avif", 1 ) )
	must( addFileToAlbum( ctx, mike, "4_webp_ll.webp", 1 ) )
	must( addFile( ctx, mike, "776AE6EC-FBF4-4549-BD58-5C442DA2860D.JPG", sql.Null[ int64 ] { } ) )
	must( addFile( ctx, mike, "IMG_2330.HEIC", sql.Null[ int64 ] { } ) )

	seagull := must1( hex.DecodeString( "cc85f99cd694c63840ff359e13610390f85c4ea0b315fc2b033e5839e7591949" ) )
	tx := must1( db.Begin() )
	defer tx.Rollback()

	qtx := queries.WithTx( tx )
	for i := 0; i < 7500; i++ {
		photo_id := must1( qtx.CreatePhoto( ctx, sqlc.CreatePhotoParams {
			Owner: justI64( mike ),
			CreatedAt: time.Now().Unix(),
			PrimaryAsset: seagull,
		} ) )

		must( qtx.AddAssetToPhoto( ctx, sqlc.AddAssetToPhotoParams {
			PhotoID: photo_id,
			AssetID: seagull,
		} ) )

		must( qtx.AddPhotoToAlbum( ctx, sqlc.AddPhotoToAlbumParams {
			AlbumID: 1,
			PhotoID: photo_id,
		} ) )
	}
	must( tx.Commit() )
}

func initFSWatcher() *fsnotify.Watcher {
	watcher := must1( fsnotify.NewWatcher() )

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if ok && event.Has( fsnotify.Create ) {
					fmt.Printf( "new file %s\n", event.Name )
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println( "error:", err )
			}
		}
	}()

	must( os.MkdirAll( "incoming", 0o755 ) )
	must( watcher.Add( "incoming" ) )

	return watcher
}

func exeChecksum() string {
	path := must1( os.Executable() )
	f := must1( os.Open( path ) )

	hasher := fnv.New64a()
	_ = must1( io.Copy( hasher, f ) )
	must( f.Close() )

	return hex.EncodeToString( hasher.Sum( nil ) )
}

func httpError( w http.ResponseWriter, status int ) {
	http.Error( w, http.StatusText( status ), status )
}

func cacheControlImmutable( w http.ResponseWriter ) {
	// 60 * 60 * 24 * 365 = 31536000
	w.Header().Set( "Cache-Control", "max-age=31536000, immutable" )
}

type User struct {
	ID int64
	Username string
}

func getChecksum( w http.ResponseWriter, r *http.Request ) {
	_ = try1( io.WriteString( w, checksum ) )
}

func accountSettings( w http.ResponseWriter, r *http.Request, user User ) {
	try( baseWithSidebar( user, r.URL.Path, "Account settings", accountSettingsTemplate() ).Render( r.Context(), w ) )
}

func getAvatar( w http.ResponseWriter, r *http.Request ) {
	sha256, err := hex.DecodeString( r.PathValue( "avatar" ) )
	if err != nil {
		httpError( w, http.StatusNotFound )
		return
	}

	avatar := queryOptional( queries.GetAvatar( r.Context(), sha256 ) )
	if !avatar.Valid {
		httpError( w, http.StatusNotFound )
		return
	}

	cacheControlImmutable( w )
	w.Header().Set( "Content-Type", "image/jpeg" )
	_ = try1( w.Write( avatar.V ) )
}

func imageToRGBA( img image.Image, err error ) ( *image.RGBA, error ) {
	if err != nil {
		return nil, err
	}

	rgba := image.NewRGBA( img.Bounds() )
	draw.Draw( rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src )
	return rgba, nil
}

func decodeImage( data []byte, extension string ) ( *image.RGBA, error ) {
	extension = strings.ToLower( extension )

	if extension == ".avif" {
		return imageToRGBA( avif.Decode( bytes.NewReader( data ) ) )
	}

	if extension == ".heic" {
		return imageToRGBA( goheif.Decode( bytes.NewReader( data ) ) )
	}

	if extension == ".webp" {
		return imageToRGBA( webp.Decode( bytes.NewReader( data ) ) )
	}

	return stb.StbLoad( data )
}

func setAvatar( w http.ResponseWriter, r *http.Request, user User ) {
	try( r.ParseMultipartForm( 32 * megabyte ) )

	// TODO: this is probably not the right way to do it...
	if len( r.MultipartForm.File[ "avatar" ] ) == 0 {
		httpError( w, http.StatusBadRequest )
		return
	}

	avatar := r.MultipartForm.File[ "avatar" ][ 0 ]
	f := try1( avatar.Open() )
	data := try1( io.ReadAll( f ) )
	try( f.Close() )

	original, err := decodeImage( data, filepath.Ext( avatar.Filename ) )
	if err != nil {
		fmt.Printf( "%v\n", err )
		httpError( w, http.StatusBadRequest )
		return
	}

	// crop to square and resize to 200x200
	min_dim := min( original.Rect.Dx(), original.Rect.Dy() )
	crop_x := original.Rect.Dx() / 2 - min_dim / 2
	crop_y := original.Rect.Dy() / 2 - min_dim / 2
	resized := stb.StbResizeAndCrop( original, crop_x, crop_y, min_dim, min_dim, 200, 200 )

	// reorient
	orientation := meta.OrientationHorizontal
	exif, err := imagemeta.Decode( bytes.NewReader( data ) )
	if err == nil {
		if exif.Orientation >= meta.OrientationHorizontal && exif.Orientation <= meta.OrientationRotate270 {
			orientation = exif.Orientation
		}
	}
	reoriented := reorient( resized, orientation )

	jpeg := try1( stb.StbToJpg( reoriented, 90 ) )

	{
		sha256 := sha256.Sum256( jpeg )

		tx := try1( db.Begin() )
		defer tx.Rollback()
		qtx := queries.WithTx( tx )

		try( qtx.AddAvatar( r.Context(), sqlc.AddAvatarParams {
			Sha256: sha256[:],
			Avatar: jpeg,
		} ) )

		try( qtx.SetUserAvatar( r.Context(), sqlc.SetUserAvatarParams {
			ID: user.ID,
			Avatar: sha256[:],
		} ) )

		try( qtx.DeleteUnusedAvatars( r.Context() ) )

		tx.Commit()
	}

	_ = try1( io.WriteString( w, green_checkmark ) )
}

func pathValueAsset( r *http.Request ) ( string, []byte, error ) {
	sha256_str := r.PathValue( "asset" )
	sha256, err := hex.DecodeString( sha256_str )
	if err != nil || len( sha256 ) != 32 {
		return "", sha256, errors.New( "asset not a sha256" )
	}
	return sha256_str, sha256, nil
}

func serveAsset( w http.ResponseWriter, r *http.Request, sha256 string, asset_type string, original_filename string ) {
	dir := "assets"
	ext := ".jpg"
	if asset_type == "heic" {
		accepts := strings.Split( r.Header.Get( "Accept" ), "," )
		if slices.Contains( accepts, "image/heic" ) {
			ext = ".heic"
		} else {
			dir = "generated"
			ext = ".heic.jpg"
		}
	}

	filename := dir + "/" + sha256 + ext
	f := try1( os.Open( filename ) )
	defer f.Close()

	cacheControlImmutable( w )
	w.Header().Set( "Content-Disposition", fmt.Sprintf( "inline; filename=\"%s%s\"", original_filename, sel( ext == ".heic.jpg", ".jpg", "" ) ) )
	w.Header().Set( "Content-Type", sel( ext == ".heic", "image/heic", "image/jpeg" ) )
	w.Header().Set( "ETag", "\"" + sha256 + "\"" )

	http.ServeContent( w, r,
		"", // filename, only used to set mime type
		time.Time { }, // modtime, assets are immutable
		f )
}

func serveThumbnail( w http.ResponseWriter, thumbnail []byte, original_filename string ) {
	cacheControlImmutable( w )
	w.Header().Set( "Content-Disposition", fmt.Sprintf( "inline; filename=\"%s_thumb.jpg\"", original_filename ) )
	w.Header().Set( "Content-Type", "image/jpeg" )
	_ = try1( w.Write( thumbnail ) )
}

func getAsset( w http.ResponseWriter, r *http.Request, user User ) {
	sha256_str, sha256, err := pathValueAsset( r )
	if err != nil {
		httpError( w, http.StatusNotFound )
		return
	}

	metadata := queryOptional( queries.GetAssetMetadata( r.Context(), sqlc.GetAssetMetadataParams {
		AssetID: sha256[:],
		Owner: justI64( user.ID ),
		Owner_2: user.ID,
		Sha256: sha256[:],
	} ) )

	if !metadata.Valid {
		httpError( w, http.StatusNotFound )
		return
	}

	if metadata.V.HasPermission == 0 {
		httpError( w, http.StatusForbidden )
		return
	}

	serveAsset( w, r, sha256_str, metadata.V.Type, metadata.V.OriginalFilename )
}

func getThumbnail( w http.ResponseWriter, r *http.Request, user User ) {
	_, sha256, err := pathValueAsset( r )
	if err != nil {
		httpError( w, http.StatusNotFound )
		return
	}

	asset := queryOptional( queries.GetAssetThumbnail( r.Context(), sha256 ) )
	if !asset.Valid {
		httpError( w, http.StatusNotFound )
		return
	}

	serveThumbnail( w, asset.V.Thumbnail, asset.V.OriginalFilename )
}

func serveJson[ T any ]( w http.ResponseWriter, x T ) {
	w.Header().Set( "Content-Type", "application/json" )
	_ = try1( w.Write( must1( json.Marshal( x ) ) ) )
}

type JsonVariant struct {
	Sha256 string
	Type string
	OriginalFilename string
	Thumbhash string
	Description *string `json:"description,omitempty"`
	DateTaken *int64 `json:"date_taken,omitempty"`
	Latitude *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

func variantsToJson( rows []sqlc.GetPhotoVariantsRow ) []JsonVariant {
	variants := make( []JsonVariant, len( rows ) )

	for i, row := range rows {
		variants[ i ] = JsonVariant {
			Sha256: hex.EncodeToString( row.Sha256 ),
			Type: row.Type,
			OriginalFilename: row.OriginalFilename,
			Thumbhash: base64.StdEncoding.EncodeToString( row.Thumbhash ),
			Description: sel( row.Description.Valid, &row.Description.String, nil ),
			DateTaken: sel( row.DateTaken.Valid, &row.DateTaken.Int64, nil ),
			Latitude: sel( row.Latitude.Valid, &row.Latitude.Float64, nil ),
			Longitude: sel( row.Longitude.Valid, &row.Longitude.Float64, nil ),
		}
	}

	return variants
}

func getPhotoMetadata( w http.ResponseWriter, r *http.Request, user User ) {
	photo_id, err := strconv.ParseInt( r.PathValue( "photo" ), 10, 64 )
	if err != nil {
		httpError( w, http.StatusBadRequest )
		return
	}

	// TODO: auth
	owner := queryOptional( queries.GetPhotoOwnerName( r.Context(), photo_id ) )
	if !owner.Valid {
		httpError( w, http.StatusNotFound )
		return
	}

	variants := try1( queries.GetPhotoVariants( r.Context(), photo_id ) )
	albums := try1( queries.GetPhotoAlbums( r.Context(), photo_id ) )

	body := struct {
		Owner string
		Variants []JsonVariant
		Albums []sqlc.GetPhotoAlbumsRow
	} {
		Owner: owner.V,
		Variants: variantsToJson( variants ),
		Albums: albums,
	}

	serveJson( w, body )
}

func geocodeRoute( w http.ResponseWriter, r *http.Request, user User ) {
	query := r.URL.Query().Get( "q" )
	if query == "" {
		httpError( w, http.StatusBadRequest )
		return
	}

	serveJson( w, geocode( query ) )
}

type HTMLAlbum struct {
	Name string
	UrlSlug string
	KeyPhotoSha256 string
}

func toHTMLAlbums( albums []sqlc.GetAlbumsForUserRow ) []HTMLAlbum {
	html_albums := make( []HTMLAlbum, len( albums ) )
	for i, album := range albums {
		html_albums[ i ] = HTMLAlbum {
			Name: album.Name,
			UrlSlug: album.UrlSlug,
			KeyPhotoSha256: hex.EncodeToString( album.KeyPhotoSha256 ),
		}
	}
	return html_albums
}

func createAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	name := r.PostFormValue( "name" )
	url := r.PostFormValue( "url" )
	shared := r.PostFormValue( "shared" ) == "1"

	try( queries.CreateAlbum( r.Context(), sqlc.CreateAlbumParams {
		Owner: user.ID,
		Name: name,
		UrlSlug: url,
		Shared: int64( sel( shared, 1, 0 ) ),
		ReadonlySecret: secureRandomBase64String( 6 ),
		ReadwriteSecret: secureRandomBase64String( 6 ),
	} ) )

	w.Header().Set( "HX-Trigger", "yougram:album_created" )

	albums := try1( queries.GetAlbumsForUser( r.Context(), user.ID ) )
	_ = try1( w.Write( must1( json.Marshal( toHTMLAlbums( albums ) ) ) ) )
}

func genericAlbumHandler( w http.ResponseWriter, r *http.Request, user User, owned_only bool, handler func( http.ResponseWriter, *http.Request, User, sqlc.GetAlbumByURLRow ) ) {
	album := queryOptional( queries.GetAlbumByURL( r.Context(), r.PathValue( "album" ) ) )
	if !album.Valid {
		httpError( w, http.StatusNotFound )
		return
	}

	if album.V.Owner != user.ID && ( owned_only || album.V.Shared == 0 ) {
		httpError( w, http.StatusForbidden )
		return
	}

	handler( w, r, user, album.V )
}

func ownedAlbumHandler( w http.ResponseWriter, r *http.Request, user User, handler func( http.ResponseWriter, *http.Request, User, sqlc.GetAlbumByURLRow ) ) {
	genericAlbumHandler( w, r, user, true, handler )
}

func sharedAlbumHandler( w http.ResponseWriter, r *http.Request, user User, handler func( http.ResponseWriter, *http.Request, User, sqlc.GetAlbumByURLRow ) ) {
	genericAlbumHandler( w, r, user, false, handler )
}

func deleteAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	ownedAlbumHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, album sqlc.GetAlbumByURLRow ) {
		const _30_days = 30 * 24 * time.Hour
		try( queries.DeleteAlbum( r.Context(), sqlc.DeleteAlbumParams {
			DeleteAt: justI64( time.Now().Add( _30_days ).Unix() ),
			UrlSlug: album.UrlSlug,
		} ) )
		w.Header().Set( "HX-Redirect", "/Special:deleted" )
	} )
}

func purgeDeletedAlbums( t time.Time ) {
	must( queries.PurgeDeletedAlbums( context.Background(), justI64( t.Unix() ) ) )
}

func updateAlbumSettings( w http.ResponseWriter, r *http.Request, user User ) {
	album_id, err := strconv.ParseInt( r.PostFormValue( "album_id" ), 10, 64 )
	if err != nil {
		httpError( w, http.StatusBadRequest )
		return
	}

	owner := queryOptional( queries.GetAlbumOwner( r.Context(), album_id ) )
	if !owner.Valid {
		http.Error( w, "No such album", http.StatusOK )
		return
	}
	if owner.V != user.ID {
		http.Error( w, "Not your album", http.StatusOK )
		return
	}

	name := r.PostFormValue( "name" )
	url := r.PostFormValue( "url" )
	if name == "" {
		_ = try1( io.WriteString( w, "Name can't be blank" ) )
		return
	}
	if url == "" {
		_ = try1( io.WriteString( w, "URL can't be blank" ) )
		return
	}

	err = queries.SetAlbumSettings( r.Context(), sqlc.SetAlbumSettingsParams {
		Name: r.PostFormValue( "name" ),
		UrlSlug: r.PostFormValue( "url" ),
		ID: album_id,
		Owner: user.ID,
	} )
	if err != nil {
		_ = try1( io.WriteString( w, err.Error() ) )
		return
	}

	w.Header().Set( "HX-Redirect", "/" + r.PostFormValue( "url" ) )
}

func checkAlbumURL( w http.ResponseWriter, r *http.Request, user User ) {
	url := r.URL.Query().Get( "url_slug" )
	exists := try1( queries.IsAlbumURLInUse( r.Context(), url ) )
	if exists == 1 {
		_ = try1( io.WriteString( w, "URL already in use" ) )
	}
}

func shareAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	album_id, err := strconv.ParseInt( r.PostFormValue( "album_id" ), 10, 64 )
	if err != nil {
		httpError( w, http.StatusBadRequest )
		return
	}

	shared, err := strconv.ParseUint( r.PostFormValue( "share" ), 10, 1 )
	if err != nil {
		httpError( w, http.StatusBadRequest )
		return
	}

	owner := queryOptional( queries.GetAlbumOwner( r.Context(), album_id ) )
	if !owner.Valid {
		httpError( w, http.StatusNotFound )
		return
	}
	if owner.V != user.ID {
		httpError( w, http.StatusForbidden )
		return
	}

	try( queries.SetAlbumIsShared( r.Context(), sqlc.SetAlbumIsSharedParams {
		Shared: int64( shared ),
		ID: album_id,
		Owner: user.ID,
	} ) )

	w.Header().Set( "HX-Trigger", sel( shared == 0, "album:stop_sharing", "album:start_sharing" ) )
}

func parsePhotoIDs( str string ) ( []int64, error ) {
	split := strings.Split( str, "," )
	ids := make( []int64, len( split ) )

	for i, id := range split {
		var err error
		ids[ i ], err = strconv.ParseInt( id, 10, 64 )
		if err != nil {
			return nil, err
		}
	}

	return ids, nil
}

func addToAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	sharedAlbumHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, album sqlc.GetAlbumByURLRow ) {
		ids, err := parsePhotoIDs( r.FormValue( "photos" ) )
		if err != nil {
			httpError( w, http.StatusBadRequest )
			return
		}

		tx := try1( db.Begin() )
		defer tx.Rollback()
		qtx := queries.WithTx( tx )

		for _, id := range ids {
			try( qtx.AddPhotoToAlbum( r.Context(), sqlc.AddPhotoToAlbumParams {
				PhotoID: id,
				AlbumID: album.ID,
			} ) )
		}

		tx.Commit()

		_ = try1( io.WriteString( w, green_checkmark ) )
	} )
}

func removeFromAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	// TODO: also let people remove their photos from albums shared with them
	// TODO: what happens when albums get unshared?
	sharedAlbumHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, album sqlc.GetAlbumByURLRow ) {
		ids, err := parsePhotoIDs( r.FormValue( "photos" ) )
		if err != nil {
			httpError( w, http.StatusBadRequest )
			return
		}

		tx := try1( db.Begin() )
		defer tx.Rollback()
		qtx := queries.WithTx( tx )

		if album.Owner == user.ID {
			for _, id := range ids {
				try( qtx.RemovePhotoFromAlbum( r.Context(), sqlc.RemovePhotoFromAlbumParams {
					PhotoID: id,
					AlbumID: album.ID,
				} ) )
			}
		} else {
			for _, id := range ids {
				try( qtx.RemoveMyPhotoFromAlbum( r.Context(), sqlc.RemoveMyPhotoFromAlbumParams {
					PhotoID: id,
					AlbumID: album.ID,
					Owner: justI64( user.ID ),
				} ) )
			}
		}

		tx.Commit()

		w.Header().Set( "HX-Refresh", "true" )
	} )
}

func serveAlbumZip( w http.ResponseWriter, r *http.Request, album sqlc.GetAlbumByURLRow ) {
	query := r.URL.Query()
	download_everything := query.Get( "variants" ) == "everything"
	download_raws := query.Get( "variants" ) != "key_only"

	rows := try1( queries.GetAlbumAssets( r.Context(), sqlc.GetAlbumAssetsParams {
		ID: album.ID,
		IncludeEverything: download_everything,
		IncludeRaws: download_raws,
	} ) )

	files := make( []ZipFile, len( rows ) )
	for i, row := range rows {
		files[ i ] = ZipFile {
			Sha256: row.Asset,
			Type: row.Type,
		}
	}

	serveZip( album.Name, files, true, w ) // TODO: heic as jpg?
}

func downloadAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	sharedAlbumHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, album sqlc.GetAlbumByURLRow ) {
		serveAlbumZip( w, r, album )
	} )
}

func downloadPhotos( w http.ResponseWriter, r *http.Request, user User ) {
	download_everything := r.FormValue( "variants" ) == "everything"
	download_raws := r.FormValue( "variants" ) != "key_only"

	ids, err := parsePhotoIDs( r.FormValue( "photos" ) )
	if err != nil {
		httpError( w, http.StatusBadRequest )
		return
	}

	var files []ZipFile
	for _, id := range ids {
		rows := try1( queries.GetPhotoAssets( r.Context(), sqlc.GetPhotoAssetsParams {
			Owner: justI64( user.ID ),
			ID: id,
			IncludeEverything: download_everything,
			IncludeRaws: download_raws,
		} ) )

		if len( rows ) == 0 {
			httpError( w, http.StatusNotFound )
			return
		}

		for _, row := range rows {
			if !row.Owned { // TODO: need to check if the photo is in a shared album too
				httpError( w, http.StatusForbidden )
				return
			}

			files = append( files, ZipFile {
				Sha256: row.Asset,
				Type: row.Type,
			} )
		}
	}

	now := time.Now().Format( "yougram-20060102-150405" )
	serveZip( now, files, true, w ) // TODO: heic as jpg?
}

func downloadPhotosAsGuest( w http.ResponseWriter, r *http.Request ) {
	guestAlbumHandler( w, r, func( w http.ResponseWriter, r *http.Request, album sqlc.GetAlbumByURLRow, writeable bool ) {
		download_everything := r.FormValue( "variants" ) == "everything"
		download_raws := r.FormValue( "variants" ) != "key_only"

		str_ids := strings.Split( r.FormValue( "photos" ), "," )
		ids := make( []int64, len( str_ids ) )

		for i, id := range str_ids {
			var err error
			ids[ i ], err = strconv.ParseInt( id, 10, 64 )
			if err != nil {
				httpError( w, http.StatusBadRequest )
				return
			}
		}

		var files []ZipFile
		for _, id := range ids {
			rows := try1( queries.GetPhotoAssetsForGuest( r.Context(), sqlc.GetPhotoAssetsForGuestParams {
				PhotoID: id,
				AlbumID: album.ID,
				ID: id,
				IncludeEverything: !download_everything,
				IncludeRaws: !download_raws,
			} ) )

			if len( rows ) == 0 {
				httpError( w, http.StatusNotFound )
				return
			}

			for _, row := range rows {
				if row.HasPermission == 0 {
					httpError( w, http.StatusForbidden )
					return
				}

				files = append( files, ZipFile {
					Sha256: row.Asset,
					Type: row.Type,
				} )
			}
		}

		now := time.Now().Format( "yougram-20060102-150405" )
		serveZip( now, files, true, w ) // TODO: heic as jpg?
	} )
}

type Photo struct {
	ID int64 `json:"id"`
	Asset string `json:"asset"`
	Thumbhash string `json:"thumbhash"`
}

func viewLibrary( w http.ResponseWriter, r *http.Request, user User ) {
	photos := []Photo { }
	for _, photo := range must1( queries.GetUserPhotos( r.Context(), justI64( user.ID ) ) ) {
		photos = append( photos, Photo {
			ID: photo.ID,
			Asset: hex.EncodeToString( photo.Sha256 ),
			Thumbhash: base64.StdEncoding.EncodeToString( photo.Thumbhash ),
		} )
	}

	body := libraryTemplate( photos )
	try( baseWithSidebar( user, r.URL.Path, "Library", body ).Render( r.Context(), w ) )
}

func viewAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	sharedAlbumHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, album sqlc.GetAlbumByURLRow ) {
		photos := []Photo { }
		for _, photo := range try1( queries.GetAlbumPhotos( r.Context(), album.ID ) ) {
			photos = append( photos, Photo {
				ID: photo.ID,
				Asset: hex.EncodeToString( photo.Sha256 ),
				Thumbhash: base64.StdEncoding.EncodeToString( photo.Thumbhash ),
			} )
		}

		album_templ := albumTemplate( album, photos, sel( album.Owner == user.ID, AlbumOwnership_Owned, AlbumOwnership_SharedWithMe ) )
		try( baseWithSidebar( user, r.URL.Path, album.Name, album_templ ).Render( r.Context(), w ) )
	} )
}

func uploadToLibrary( w http.ResponseWriter, r *http.Request, user User ) {
	try( r.ParseMultipartForm( 100 * megabyte ) )

	if len( r.MultipartForm.File[ "assets" ] ) == 0 {
		httpError( w, http.StatusBadRequest )
		return
	}

	assets := make( []AddedAsset, len( r.MultipartForm.File[ "assets" ] ) )

	for i, header := range r.MultipartForm.File[ "assets" ] {
		f := try1( header.Open() )
		assets[ i ] = try1( addAsset( r.Context(), try1( io.ReadAll( f ) ), header.Filename ) )
		try( f.Close() )
	}

	var photo_id sql.Null[ int64 ]
	for _, asset := range assets {
		photos := try1( queries.GetAssetPhotos( r.Context(), sqlc.GetAssetPhotosParams {
			AssetID: asset.Sha256[:],
			Owner: justI64( user.ID ),
		} ) )

		if len( photos ) == 0 {
			continue
		}

		if len( photos ) == 1 && !photo_id.Valid {
			photo_id = just( photos[ 0 ] )
			continue
		}

		httpError( w, http.StatusConflict )
		return
	}

	tx := try1( db.Begin() )
	defer tx.Rollback()
	qtx := queries.WithTx( tx )

	if !photo_id.Valid {
		photo_id = just( try1( qtx.CreatePhoto( r.Context(), sqlc.CreatePhotoParams {
			Owner: justI64( user.ID ),
			CreatedAt: time.Now().Unix(),
			PrimaryAsset: assets[ 0 ].Sha256[:],
		} ) ) )
	}

	for _, asset := range assets {
		try( qtx.AddAssetToPhoto( r.Context(), sqlc.AddAssetToPhotoParams {
			AssetID: asset.Sha256[:],
			PhotoID: photo_id.V,
		} ) )
	}

	tx.Commit()
}

func uploadToAlbumImpl( w http.ResponseWriter, r *http.Request, userID sql.NullInt64, album sqlc.GetAlbumByURLRow ) {
	try( r.ParseMultipartForm( 100 * megabyte ) )

	if len( r.MultipartForm.File[ "assets" ] ) == 0 {
		httpError( w, http.StatusBadRequest )
		return
	}

	assets := make( []AddedAsset, len( r.MultipartForm.File[ "assets" ] ) )

	for i, header := range r.MultipartForm.File[ "assets" ] {
		f := try1( header.Open() )
		assets[ i ] = try1( addAsset( r.Context(), try1( io.ReadAll( f ) ), header.Filename ) )
		try( f.Close() )
	}

	var photo_id sql.Null[ int64 ]
	for _, asset := range assets {
		photos := try1( queries.GetAssetPhotos( r.Context(), sqlc.GetAssetPhotosParams {
			AssetID: asset.Sha256[:],
			Owner: userID,
		} ) )

		if len( photos ) == 0 {
			continue
		}

		if len( photos ) == 1 && !photo_id.Valid {
			photo_id = just( photos[ 0 ] )
			continue
		}

		httpError( w, http.StatusConflict )
		return
	}

	tx := try1( db.Begin() )
	defer tx.Rollback()
	qtx := queries.WithTx( tx )

	if !photo_id.Valid {
		photo_id = just( try1( qtx.CreatePhoto( r.Context(), sqlc.CreatePhotoParams {
			Owner: userID,
			CreatedAt: time.Now().Unix(),
			PrimaryAsset: assets[ 0 ].Sha256[:],
		} ) ) )
	}

	for _, asset := range assets {
		try( qtx.AddAssetToPhoto( r.Context(), sqlc.AddAssetToPhotoParams {
			AssetID: asset.Sha256[:],
			PhotoID: photo_id.V,
		} ) )
	}

	try( qtx.AddPhotoToAlbum( r.Context(), sqlc.AddPhotoToAlbumParams {
		PhotoID: photo_id.V,
		AlbumID: album.ID,
	} ) )

	tx.Commit()
}

func uploadToAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	sharedAlbumHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, album sqlc.GetAlbumByURLRow ) {
		uploadToAlbumImpl( w, r, justI64( user.ID ), album )
	} )
}

func uploadToPhoto( w http.ResponseWriter, r *http.Request, user User ) {
	pathPhotoHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, photo_id int64 ) {
		try( r.ParseMultipartForm( 100 * megabyte ) )

		if len( r.MultipartForm.File[ "assets" ] ) == 0 {
			httpError( w, http.StatusBadRequest )
			return
		}

		assets := make( []AddedAsset, len( r.MultipartForm.File[ "assets" ] ) )

		for i, header := range r.MultipartForm.File[ "assets" ] {
			f := try1( header.Open() )
			assets[ i ] = try1( addAsset( r.Context(), try1( io.ReadAll( f ) ), header.Filename ) )
			try( f.Close() )
		}

		tx := try1( db.Begin() )
		defer tx.Rollback()
		qtx := queries.WithTx( tx )

		for _, asset := range assets {
			try( qtx.AddAssetToPhoto( r.Context(), sqlc.AddAssetToPhotoParams {
				AssetID: asset.Sha256[:],
				PhotoID: photo_id,
			} ) )
		}

		tx.Commit()
	} )
}

func getAssetAsGuest( w http.ResponseWriter, r *http.Request ) {
	sha256_str, sha256, err := pathValueAsset( r )
	if err != nil {
		httpError( w, http.StatusNotFound )
		return
	}

	secret := r.PathValue( "secret" )
	metadata := queryOptional( queries.GetAssetGuestMetadata( r.Context(), sqlc.GetAssetGuestMetadataParams {
		UrlSlug: r.PathValue( "album" ),
		ReadonlySecret: secret,
		ReadwriteSecret: secret,
		Sha256: sha256[:],
	} ) )
	if !metadata.Valid {
		httpError( w, http.StatusNotFound )
		return
	}
	if metadata.V.HasPermission == 0 {
		httpError( w, http.StatusForbidden )
		return
	}

	serveAsset( w, r, sha256_str, metadata.V.Type, metadata.V.OriginalFilename )
}

func getThumbnailAsGuest( w http.ResponseWriter, r *http.Request ) {
	_, sha256, err := pathValueAsset( r )
	if err != nil {
		httpError( w, http.StatusNotFound )
		return
	}

	secret := r.PathValue( "secret" )
	asset := queryOptional( queries.GetAssetGuestThumbnail( r.Context(), sqlc.GetAssetGuestThumbnailParams {
		UrlSlug: r.PathValue( "album" ),
		ReadonlySecret: secret,
		ReadwriteSecret: secret,
		Sha256: sha256[:],
	} ) )
	if !asset.Valid {
		httpError( w, http.StatusNotFound )
		return
	}
	if asset.V.HasPermission == 0 {
		httpError( w, http.StatusForbidden )
		return
	}

	serveThumbnail( w, asset.V.Thumbnail, asset.V.OriginalFilename )
}

func guestAlbumHandler( w http.ResponseWriter, r *http.Request, handler func( http.ResponseWriter, *http.Request, sqlc.GetAlbumByURLRow, bool ) ) {
	album := queryOptional( queries.GetAlbumByURL( r.Context(), r.PathValue( "album" ) ) )
	secret := r.PathValue( "secret" )
	if !album.Valid || ( secret != album.V.ReadonlySecret && secret != album.V.ReadwriteSecret ) {
		httpError( w, http.StatusForbidden )
		return
	}

	handler( w, r, album.V, secret == album.V.ReadwriteSecret )
}

func viewAlbumAsGuest( w http.ResponseWriter, r *http.Request ) {
	guestAlbumHandler( w, r, func( w http.ResponseWriter, r *http.Request, album sqlc.GetAlbumByURLRow, writeable bool ) {
		photos := []Photo { }
		for _, photo := range try1( queries.GetAlbumPhotos( r.Context(), album.ID ) ) {
			photos = append( photos, Photo {
				ID: photo.ID,
				Asset: hex.EncodeToString( photo.Sha256 ),
				Thumbhash: base64.StdEncoding.EncodeToString( photo.Thumbhash ),
			} )
		}

		album_templ := guestAlbumTemplate( album, photos, writeable )
		try( guestBase( album.Name, album_templ ).Render( r.Context(), w ) )
	} )
}

func downloadAlbumAsGuest( w http.ResponseWriter, r *http.Request ) {
	guestAlbumHandler( w, r, func( w http.ResponseWriter, r *http.Request, album sqlc.GetAlbumByURLRow, writeable bool ) {
		serveAlbumZip( w, r, album )
	} )
}

func uploadToAlbumAsGuest( w http.ResponseWriter, r *http.Request ) {
	guestAlbumHandler( w, r, func( w http.ResponseWriter, r *http.Request, album sqlc.GetAlbumByURLRow, writeable bool ) {
		if !writeable {
			httpError( w, http.StatusForbidden )
			return
		}

		uploadToAlbumImpl( w, r, sql.NullInt64 { }, album )
	} )
}

type LatLong struct {
	Latitude float64
	Longitude float64
}

func degToRad( d float64 ) float64 {
	return d * math.Pi / 180.0
}

func angleDiff( a float64, b float64 ) float64 {
	d := math.Mod( math.Abs( a - b ), 360.0 )
	if d > 180 {
		d = 360 - d
	}
	return d
}

func distance( a LatLong, b LatLong ) float64 {
	const earth_radius float64 = 6371
	dlat := degToRad( angleDiff( a.Latitude, b.Latitude ) )
	dlong := degToRad( angleDiff( a.Longitude, b.Longitude ) )
	return earth_radius * math.Acos( math.Cos( dlat ) * math.Cos( dlong ) )
}

func reorient( img *image.RGBA, orientation meta.Orientation ) *image.RGBA {
	if orientation == meta.OrientationHorizontal {
		return img
	}

	type Walker struct {
		Origin image.Point
		Dx image.Point
		Dy image.Point
	}

	var walkers = map[ meta.Orientation ] Walker {
		meta.OrientationMirrorHorizontal:          { image.Point { -1, +1 }, image.Point { -1, +0 }, image.Point { +0, +1 } },
		meta.OrientationRotate180:                 { image.Point { -1, -1 }, image.Point { -1, +0 }, image.Point { +0, -1 } },
		meta.OrientationMirrorVertical:            { image.Point { +1, -1 }, image.Point { +1, +0 }, image.Point { +0, -1 } },
		meta.OrientationMirrorHorizontalRotate270: { image.Point { +1, +1 }, image.Point { +0, +1 }, image.Point { +1, +0 } },
		meta.OrientationRotate90:                  { image.Point { -1, +1 }, image.Point { +0, +1 }, image.Point { -1, +0 } },
		meta.OrientationMirrorHorizontalRotate90:  { image.Point { -1, -1 }, image.Point { +0, -1 }, image.Point { -1, +0 } },
		meta.OrientationRotate270:                 { image.Point { +1, -1 }, image.Point { +0, -1 }, image.Point { +1, +0 } },
	}

	walker := walkers[ orientation ]
	swapdims := walker.Dx.X == 0
	reoriented := image.NewRGBA( image.Rect( 0, 0, sel( swapdims, img.Rect.Dy(), img.Rect.Dx() ), sel( swapdims, img.Rect.Dx(), img.Rect.Dy() ) ) )
	origin := image.Point {
		X: sel( walker.Origin.X == 1, 0, reoriented.Rect.Dx() - 1 ),
		Y: sel( walker.Origin.Y == 1, 0, reoriented.Rect.Dy() - 1 ),
	}

	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			cursor := origin.Add( walker.Dx.Mul( x ) ).Add( walker.Dy.Mul( y ) )
			reoriented.SetRGBA( cursor.X, cursor.Y, img.RGBAAt( x, y ) )
		}
	}

	return reoriented
}

func generateThumbnail( image *image.RGBA ) ( []byte, []byte ) {
	const thumbnail_size = 512.0

	scale := min( 1, thumbnail_size / float64( min( image.Rect.Dx(), image.Rect.Dy() ) ) )
	thumbnail := stb.StbResize( image, int( float64( image.Rect.Dx() ) * scale ), int( float64( image.Rect.Dy() ) * scale ) )
	thumbnail_jpg := must1( stb.StbToJpg( thumbnail, 75 ) )

	return thumbnail_jpg, thumbhash.EncodeImage( thumbnail )
}

func writeFileFSync( name string, data []byte, perm os.FileMode ) error {
	f, err := os.OpenFile( name, os.O_WRONLY | os.O_CREATE | os.O_TRUNC, perm )
	if err != nil {
		return err
	}
	_, err = f.Write( data )
	err1 := f.Sync()
	err2 := f.Close()
	return cmp.Or( err, err1, err2 )
}

func saveAsset( data []byte, filename string ) error {
	return writeFileFSync( "assets/" + filename, data, 0644 )
}

func saveGenerated( data []byte, filename string ) error {
	return os.WriteFile( "generated/" + filename, data, 0644 )
}

type AddedAsset struct {
	Sha256 [32]byte
	Date sql.NullInt64
	Latitude sql.NullFloat64
	Longitude sql.NullFloat64
}

func addAsset( ctx context.Context, data []byte, filename string ) ( AddedAsset, error ) {
	fmt.Printf( "addAsset( %s )\n", filename )

	sha256 := sha256.Sum256( data )

	var date sql.NullInt64
	var latitude sql.NullFloat64
	var longitude sql.NullFloat64
	orientation := meta.OrientationHorizontal

	exif, err := imagemeta.Decode( bytes.NewReader( data ) )
	if err == nil {
		if exif.Orientation >= meta.OrientationHorizontal && exif.Orientation <= meta.OrientationRotate270 {
			orientation = exif.Orientation
		}

		if !exif.CreateDate().IsZero() {
			date = justI64( exif.CreateDate().Unix() )
		}

		if exif.GPS.Latitude() != 0 || exif.GPS.Longitude() != 0 || exif.GPS.Altitude() != 0 {
			latitude = sql.NullFloat64 { exif.GPS.Latitude(), true }
			longitude = sql.NullFloat64 { exif.GPS.Longitude(), true }
		}
	}

	if try1( queries.AssetExists( ctx, sha256[:] ) ) == 1 {
		// TODO: get metadata from the db maybe
		return AddedAsset { sha256, date, latitude, longitude }, nil
	}

	before := time.Now()

	extension := strings.ToLower( filepath.Ext( filename ) )
	is_heic := strings.ToLower( filepath.Ext( filename ) ) == ".heic"
	ext := sel( is_heic, ".heic", ".jpg" )

	decoded, err := decodeImage( data, extension )
	if err != nil {
		return AddedAsset { }, err
	}

	fmt.Printf( "\torientation %d\n", orientation )

	// if !album_id.Valid && date.Valid && latitude.Valid {
	// 	rows := try1( db.Query( "SELECT album_id, latitude, longitude, radius FROM auto_assign_rules WHERE start_date <= $1 AND end_date >= $1 ORDER BY end_date - start_date ASC", date.V.Unix() ) )
	// 	defer rows.Close()
    //
	// 	for rows.Next() {
	// 		var id int64
	// 		var rule_latitude float64
	// 		var rule_longitude float64
	// 		var radius float64
	// 		try( rows.Scan( &id, &rule_latitude, &rule_longitude, &radius ) )
    //
	// 		if distance( LatLong { latitude.V, longitude.V }, LatLong { rule_latitude, rule_longitude } ) > radius {
	// 			continue
	// 		}
    //
	// 		album_id = sql.Null[ int64 ] { id, true }
	// 		break
	// 	}
	// }

	fmt.Printf( "\tEXIF decoded %dms\n", time.Since( before ).Milliseconds() )

	reoriented := reorient( decoded, orientation )
	fmt.Printf( "\treoriented %dms\n", time.Since( before ).Milliseconds() )

	hex_sha256 := hex.EncodeToString( sha256[:] )
	err = saveAsset( data, hex_sha256 + ext )
	if err != nil {
		return AddedAsset { }, err
	}
	if is_heic {
		jpeg := must1( stb.StbToJpg( reoriented, 95 ) )
		fmt.Printf( "\theic -> jpeg %dms %d -> %d\n", time.Since( before ).Milliseconds(), len( data ), len( jpeg ) )
		err = saveGenerated( jpeg, hex_sha256 + ".heic.jpg" )
		if err != nil {
			return AddedAsset { }, err
		}
	}
	fmt.Printf( "\tsave assets %dms\n", time.Since( before ).Milliseconds() )

	thumbnail, thumbhash := generateThumbnail( reoriented )

	err = queries.CreateAsset( ctx, sqlc.CreateAssetParams {
		Sha256: sha256[:],
		CreatedAt: time.Now().Unix(),
		OriginalFilename: filename,
		Type: sel( is_heic, "heic", "jpg" ),
		Thumbnail: thumbnail,
		Thumbhash: thumbhash,
		DateTaken: date,
		Latitude: latitude,
		Longitude: longitude,
	} )

	fmt.Printf( "\tdone %dms\n", time.Since( before ).Milliseconds() )

	return AddedAsset { sha256, date, latitude, longitude }, err
}

func addFile( ctx context.Context, user int64, path string, album_id sql.Null[ int64 ] ) error {
	f := must1( os.Open( path ) )
	defer f.Close()
	img := must1( io.ReadAll( f ) )
	asset, err := addAsset( ctx, img, path )
	if err != nil {
		return err
	}

	photos, err := queries.GetAssetPhotos( ctx, sqlc.GetAssetPhotosParams {
		AssetID: asset.Sha256[:],
		Owner: justI64( user ),
	} )
	if err != nil {
		return err
	}

	if len( photos ) == 0 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		qtx := queries.WithTx( tx )

		photo_id, err := qtx.CreatePhoto( ctx, sqlc.CreatePhotoParams {
			Owner: justI64( user ),
			CreatedAt: time.Now().Unix(),
			PrimaryAsset: asset.Sha256[:],
		} )
		if err != nil {
			return err
		}

		err = qtx.AddAssetToPhoto( ctx, sqlc.AddAssetToPhotoParams {
			PhotoID: photo_id,
			AssetID: asset.Sha256[:],
		} )
		if err != nil {
			return err
		}

		if album_id.Valid {
			err = qtx.AddPhotoToAlbum( ctx, sqlc.AddPhotoToAlbumParams {
				AlbumID: album_id.V,
				PhotoID: photo_id,
			} )
			if err != nil {
				return err
			}
		}

		return tx.Commit()
	}

	return nil
}

func addFileToAlbum( ctx context.Context, user int64, path string, album_id int64 ) error {
	return addFile( ctx, user, path, sql.Null[ int64 ] { album_id, true } )
}

func pathPhotoHandler( w http.ResponseWriter, r *http.Request, user User, handler func( http.ResponseWriter, *http.Request, User, int64 ) ) {
	photo_id, err := strconv.ParseInt( r.PostFormValue( "photo" ), 10, 64 )
	if err != nil {
		httpError( w, http.StatusBadRequest )
		return
	}

	owner := queryOptional( queries.GetPhotoOwner( r.Context(), photo_id ) )
	if !owner.Valid {
		httpError( w, http.StatusNotFound )
		return
	}
	if !owner.V.Valid || owner.V.Int64 != user.ID {
		httpError( w, http.StatusForbidden )
		return
	}

	handler( w, r, user, photo_id )
}

type ZipFile struct {
	Sha256 []byte
	Type string
}

func serveZip( filename string, assets []ZipFile, heic_as_jpeg bool, w http.ResponseWriter ) {
	// magic numbers obtained from ImHex and Wikipedia
	const end_of_central_directory_size = 22
	content_length := int64( end_of_central_directory_size )
	for _, asset := range assets {
		dir := "assets"
		disk_extension := asset.Type
		zip_extension := asset.Type
		if heic_as_jpeg && asset.Type == "heic" {
			dir = "generated"
			disk_extension = "heic.jpg"
			zip_extension = "jpg"
		}

		filename := hex.EncodeToString( asset.Sha256 )
		f := try1( os.Open( dir + "/" + filename + "." + disk_extension ) )
		info := try1( f.Stat() )
		try( f.Close() )

		const local_file_header_size = 30
		const central_directory_entry_size = 46
		const data_descriptor_size = 16
		content_length += local_file_header_size + central_directory_entry_size + data_descriptor_size
		content_length += info.Size()
		content_length += 2 * int64( len( filename + "." + zip_extension ) )
	}

	w.Header().Set( "Content-Disposition", fmt.Sprintf( "attachment; filename=\"%s.zip\"", filename ) )
	w.Header().Set( "Content-Type", "application/zip" )
	w.Header().Set( "Content-Length", strconv.FormatInt( content_length, 10 ) )

	archive := zip.NewWriter( w )

	for _, asset := range assets {
		dir := "assets"
		disk_extension := asset.Type
		zip_extension := asset.Type
		if heic_as_jpeg && asset.Type == "heic" {
			dir = "generated"
			disk_extension = "heic.jpg"
			zip_extension = "jpg"
		}

		filename := hex.EncodeToString( asset.Sha256 )
		a := try1( os.Open( dir + "/" + filename + "." + disk_extension ) )

		z := try1( archive.CreateHeader( &zip.FileHeader {
			Name: filename + "." + zip_extension,
			Method: zip.Store,
		} ) )
		_ = try1( io.Copy( z, a ) )
		try( a.Close() )
	}

	try( archive.Close() )
}

func serveString( content string, content_type string ) func( http.ResponseWriter, *http.Request ) {
	return func( w http.ResponseWriter, r *http.Request ) {
		cacheControlImmutable( w )
		w.Header().Set( "Content-Type", content_type )
		_ = try1( io.WriteString( w, content ) )
	}
}

func serveJS( content string ) func( http.ResponseWriter, *http.Request ) {
	return serveString( content, "text/javascript; charset=utf-8" )
}

func makeRouteRegex( route string ) *regexp.Regexp {
	regex := "^" + route + "$"
	regex = strings.ReplaceAll( regex, ".", "\\." ) // escape .
	regex = strings.ReplaceAll( regex, "{", "(?P<" ) // convert {} to named captures
	regex = strings.ReplaceAll( regex, "}", ">[^:/]+)" )
	return regexp.MustCompile( regex )
}

type Route struct {
	Method string
	Route string
	Handler func( http.ResponseWriter, *http.Request )
}

func startHttpServer( addr string, _404_to_403 bool, routes []Route ) *http.Server {
	regexes := make( []*regexp.Regexp, len( routes ) )
	for i, route := range routes {
		regexes[ i ] = makeRouteRegex( route.Route )
	}

	mux := http.NewServeMux()
	mux.HandleFunc( "/", func( w http.ResponseWriter, r *http.Request ) {
		// start := time.Now()
		defer func() {
			// dt := time.Now().Sub( start )
			// if dt.Milliseconds() >= 2 {
			// 	fmt.Printf( "Request took > 2ms (%f): %s\n", float64( dt.Microseconds() ) / 1000, r.URL.Path )
			// }
			if r := recover(); r != nil {
				log.Print( r )
				httpError( w, http.StatusInternalServerError )
				return
			}
		}()

		is405 := false

		for i, route := range routes {
			if matches := regexes[ i ].FindStringSubmatch( r.URL.Path ); len( matches ) > 0 {
				if r.Method == route.Method {
					for j, name := range regexes[ i ].SubexpNames()[ 1: ] {
						r.SetPathValue( name, matches[ j + 1 ] )
					}
					route.Handler( w, r )
					return
				}

				is405 = true
			}
		}

		if is405 {
			httpError( w, http.StatusMethodNotAllowed )
		} else {
			httpError( w, sel( _404_to_403, http.StatusForbidden, http.StatusNotFound ) )
		}
	} )

	http_server := &http.Server{
		Addr: addr,
		Handler: mux,
	}

	go func() {
		err := http_server.ListenAndServe()
		if err != nil && !errors.Is( err, http.ErrServerClosed ) {
			log.Fatal( err )
		}
	}()

	return http_server
}

func showHelpAndQuit() {
	fmt.Printf(
`Usage: %s <command>
    serve --private <addr:port> --guest <addr:port> --guest-url <https://guestgram.blah.com>
    create-user [username]
        Create a user with the given username and a random password.
    reset-password [username]
        Reset the given user's password to a random password.
    disable-user [username]
        Disable the given user account.
    enable-user [username]
        Re-enables a disabled account.
`, os.Args[ 0 ] )
	os.Exit( 1 )
}

func main() {
	runtime.GOMAXPROCS( 2 )

	checksum = exeChecksum()

	{
		_, err := os.Stat( "you_did_not_bind_a_data_volume" ) // see also container/prepare_container.zig
		if err == nil || !errors.Is( err, os.ErrNotExist ) {
			fmt.Printf( "Start the container with a volume mounted at /data, i.e. podman run -v /a/b/c:/data ...\n" )
			os.Exit( 1 )
		}
	}

	must( os.MkdirAll( "assets", 0o755 ) )
	must( os.MkdirAll( "generated", 0o755 ) )

	db_path := "yougram.sq3"
	private_listen_addr := "0.0.0.0:5678"
	guest_listen_addr := "0.0.0.0:5679"
	guest_url = "http://localhost:5679"
	no_args := len( os.Args ) == 1

	initCookieAEAD( no_args )

	if no_args {
		if IsReleaseBuild {
			showHelpAndQuit()
		}

		db_path = "file::memory:"
		fmt.Println( "Using in memory database. Nothing will be saved when you quit the server!" )
		fmt.Printf( "curl --header \"Cookie: auth=%s\" localhost:5678/\n", encodeAuthCookie( "mike", make( []byte, 16 ) ) )
	}

	// sqlite_vec.Auto()
	db = must1( sql.Open( "sqlite3", db_path + "?cache=shared" ) )
	defer db.Close()

	queries = sqlc.New( db )

	initDB( len( os.Args ) == 1 )

	if len( os.Args ) > 1 {
		switch os.Args[ 1 ] {
		case "serve":
			flags := flag.NewFlagSet( "serve", flag.ExitOnError )
			private_addr_flag := flags.String( "private-listen-addr", "", "The listen address for yougram's private interface. This should probably be behind a VPN." )
			guest_addr_flag := flags.String( "guest-listen-addr", "", "The listen address for yougram's guest interface. This is intended to be publically accessible, an easy way to do that is Cloudflare Tunnel or Tailscale Funnel." )
			guest_url_flag := flags.String( "guest-url", guest_url, "The public URL for the guest interface, so links from the private interface work." )

			must( flags.Parse( os.Args[ 2: ] ) )

			if *private_addr_flag == "" || *guest_addr_flag == "" || *guest_url_flag == "" {
				fmt.Println( "You need to provide all of these arguments:" )
				flags.PrintDefaults()
				os.Exit( 1 )
			}

			private_listen_addr = *private_addr_flag
			guest_listen_addr = *guest_addr_flag
			guest_url = *guest_url_flag

		case "create-user":
			if len( os.Args ) != 3 {
				showHelpAndQuit()
			}
			password := secureRandomHexString( 4 )
			_ = must1( queries.CreateUser( context.Background(), sqlc.CreateUserParams {
				Username: unicodeNormalize( os.Args[ 2 ] ),
				Password: hashPassword( password ),
				Cookie: secureRandomBytes( 16 ),
			} ) )
			fmt.Printf( "Created user %s, their password is %s\n", os.Args[ 2 ], password )
			os.Exit( 0 )

		case "reset-password":
			if len( os.Args ) != 3 {
				showHelpAndQuit()
			}
			password := secureRandomHexString( 4 )
			must( queries.ResetUserPassword( context.Background(), sqlc.ResetUserPasswordParams {
				Username: unicodeNormalize( os.Args[ 2 ] ),
				Password: hashPassword( password ),
				Cookie: secureRandomBytes( 16 ),
			} ) )
			fmt.Printf( "Reset %s's password to %s\n", os.Args[ 2 ], password )
			os.Exit( 0 )

		case "disable-user":
			if len( os.Args ) != 3 {
				showHelpAndQuit()
			}
			must( queries.DisableUser( context.Background(), unicodeNormalize( os.Args[ 2 ] ) ) )
			os.Exit( 0 )

		case "enable-user":
			if len( os.Args ) != 3 {
				showHelpAndQuit()
			}
			must( queries.EnableUser( context.Background(), unicodeNormalize( os.Args[ 2 ] ) ) )
			os.Exit( 0 )

		default: showHelpAndQuit()
		}
	}

	if must1( queries.AreThereAnyUsers( context.Background() ) ) == 0 {
		fmt.Printf( "You need to create a user by running \"%s create-user\" first!\n", os.Args[ 0 ] )
		os.Exit( 1 )
	}

	fs_watcher := initFSWatcher()
	defer fs_watcher.Close()

	moondream.Init()
	initBackgroundTaskRunner()
	initGeocoder()

	// daily tasks
	go func() {
		for now := range time.Tick( 24 * time.Hour ) {
			purgeDeletedAlbums( now )
		}
	}()

	private_http_server := startHttpServer( private_listen_addr, false, []Route {
		{ "GET",  "/Special:checksum", getChecksum },
		{ "GET",  "/Special:alpinejs-3.14.9.js", serveJS( alpinejs ) },
		{ "GET",  "/Special:htmx-2.0.4.js", serveJS( htmxjs ) },
		{ "GET",  "/Special:thumbhash-1.0.0.js", serveJS( thumbhashjs ) },

		{ "POST", "/Special:authenticate", authenticate },
		{ "GET",  "/Special:logout", logout },

		{ "GET",  "/Special:account", requireAuth( accountSettings ) },
		{ "GET",  "/Special:avatar/{avatar}", getAvatar },
		{ "POST", "/Special:avatar", requireAuth( setAvatar ) },
		{ "POST", "/Special:password", requireAuth( setPassword ) },
		{ "POST", "/Special:resetPassword", resetPassword },

		{ "GET",  "/Special:asset/{asset}", requireAuth( getAsset ) },
		{ "GET",  "/Special:thumbnail/{asset}", requireAuth( getThumbnail ) },
		{ "GET",  "/Special:photoMetadata/{photo}", requireAuth( getPhotoMetadata ) },
		{ "GET",  "/Special:geocode", requireAuthNoLoginForm( geocodeRoute ) },

		{ "PUT",  "/Special:createAlbum", requireAuth( createAlbum ) },
		{ "POST", "/Special:albumSettings", requireAuth( updateAlbumSettings ) },
		{ "GET",  "/Special:checkAlbumURL", requireAuth( checkAlbumURL ) },
		{ "POST", "/Special:shareAlbum", requireAuth( shareAlbum ) },
		{ "DELETE", "/{album}", requireAuth( deleteAlbum ) },

		{ "PUT",  "/Special:addToAlbum/{album}", requireAuth( addToAlbum ) },
		{ "PUT",  "/Special:removeFromAlbum/{album}", requireAuth( removeFromAlbum ) },

		{ "GET",  "/Special:download/{album}", requireAuth( downloadAlbum ) },
		{ "POST", "/Special:download", requireAuth( downloadPhotos ) },

		{ "GET",  "/", requireAuth( viewLibrary ) },
		{ "GET",  "/{album}", requireAuth( viewAlbum ) },

		{ "PUT",  "/", requireAuth( uploadToLibrary ) },
		{ "PUT",  "/{album}", requireAuth( uploadToAlbum ) },
		{ "PUT",  "/Special:uploadToPhoto", requireAuth( uploadToPhoto ) },
	} )

	guest_http_server := startHttpServer( guest_listen_addr, true, []Route {
		{ "GET",  "/Special:checksum", getChecksum },
		{ "GET",  "/Special:alpinejs-3.14.9.js", serveJS( alpinejs ) },
		{ "GET",  "/Special:htmx-2.0.4.js", serveJS( htmxjs ) },
		{ "GET",  "/Special:thumbhash-1.0.0.js", serveJS( thumbhashjs ) },
		{ "GET",  "/robots.txt", serveString( "User-agent: *\nDisallow: /", "text/plain" ) },

		{ "GET",  "/{album}/{secret}", viewAlbumAsGuest },
		{ "GET",  "/{album}/{secret}/asset/{asset}", getAssetAsGuest },
		{ "GET",  "/{album}/{secret}/thumbnail/{asset}", getThumbnailAsGuest },

		{ "GET",  "/{album}/{secret}/download", downloadAlbumAsGuest },
		{ "POST", "/{album}/{secret}/download", downloadPhotosAsGuest },
		{ "PUT",  "/{album}/{secret}", uploadToAlbumAsGuest },
	} )

	done := make( chan os.Signal, 1 )
	signal.Notify( done, syscall.SIGINT, syscall.SIGTERM )
	<-done

	ctx, cancel := context.WithTimeout( context.Background(), 5 * time.Second )
	defer func() {
		db.Close()
		cancel()
	}()

	must( private_http_server.Shutdown( ctx ) )
	must( guest_http_server.Shutdown( ctx ) )

	shutdownGeocoder()
	shutdownBackgroundTaskRunner()
	moondream.Shutdown()
}
