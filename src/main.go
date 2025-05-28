package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	"mikegram/sqlc"
	"mikegram/stb"

	"golang.org/x/text/unicode/norm"
	"github.com/adrium/goheif"
	"github.com/galdor/go-thumbhash"
	"github.com/evanoberholster/imagemeta"
	"github.com/evanoberholster/imagemeta/exif2"
	"github.com/evanoberholster/imagemeta/meta"
	"github.com/tdewolff/minify/v2"
	"github.com/fsnotify/fsnotify"
	minify_js "github.com/tdewolff/minify/v2/js"
	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB
var queries *sqlc.Queries

//go:embed schema.sql
var db_schema string

//go:embed vendor_js/alpine-3.14.9.js
var alpinejs string
//go:embed vendor_js/fuzzysort-3.1.0.js
var fuzzysortjs string
//go:embed vendor_js/htmx-2.0.4.js
var htmxjs string
//go:embed vendor_js/thumbhash.js
var thumbhashjs string

var minifier *minify.M

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

func query( ctx context.Context, query string, args ...interface{} ) error {
	_, err := db.ExecContext( ctx, query, args... )
	return err
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

func queryOptional[ T any ]( row T, err error ) sql.Null[ T ] {
	if err != nil {
		if errors.Is( err, sql.ErrNoRows ) {
			return sql.Null[ T ] { }
		}
		panic( err )
	}
	return just( row )
}

func initDB() {
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

	mike := must1( queries.CreateUser( ctx, sqlc.CreateUserParams {
		Username: norm.NFKC.String( "mike" ),
		Password: norm.NFKC.String( "gg" ),
		Cookie: "123",
	} ) )

	_ = must1( queries.CreateUser( ctx, sqlc.CreateUserParams {
		Username: norm.NFKC.String( "mum" ),
		Password: norm.NFKC.String( "gg" ),
		Cookie: "123",
	} ) )

	_ = must1( queries.CreateUser( ctx, sqlc.CreateUserParams {
		Username: norm.NFKC.String( "dad" ),
		Password: norm.NFKC.String( "gg" ),
		Cookie: "123",
	} ) )

	must( queries.CreateAlbum( ctx, sqlc.CreateAlbumParams {
		Owner: 1,
		Name: "France 2024",
		UrlSlug: "france-2024",
		Shared: 1,
		ReadonlySecret: "aaaaaaaa",
		ReadwriteSecret: "bbbbbbbb",
	} ) )
	must( queries.CreateAlbum( ctx, sqlc.CreateAlbumParams {
		Owner: 1,
		Name: "Helsinki 2024",
		UrlSlug: "helsinki-2024",
		Shared: 0,
		ReadonlySecret: "aaaaaaaa",
		ReadwriteSecret: "bbbbbbbb",
		AutoassignStartDate: sql.NullInt64 { time.Date( 2024, time.January, 1, 0, 0, 0, 0, time.UTC ).Unix(), true },
		AutoassignEndDate: sql.NullInt64 { time.Date( 2024, time.December, 31, 0, 0, 0, 0, time.UTC ).Unix(), true },
		AutoassignLatitude: sql.NullFloat64 { 60.1699, true },
		AutoassignLongitude: sql.NullFloat64 { 24.9384, true },
		AutoassignRadius: sql.NullFloat64 { 50, true },
	} ) )

	must( addFileToAlbum( ctx, mike, "DSCN0025.jpg", 2 ) )
	must( addFileToAlbum( ctx, mike, "DSCF2994.jpeg", 1 ) )
	must( addFile( ctx, mike, "776AE6EC-FBF4-4549-BD58-5C442DA2860D.JPG", sql.Null[ int64 ] { } ) )
	must( addFile( ctx, mike, "IMG_2330.HEIC", sql.Null[ int64 ] { } ) )

	seagull := must1( hex.DecodeString( "cc85f99cd694c63840ff359e13610390f85c4ea0b315fc2b033e5839e7591949" ) )
	tx := must1( db.Begin() )
	defer tx.Rollback()

	qtx := queries.WithTx( tx )
	for i := 0; i < 7500; i++ {
		photo_id := must1( qtx.CreatePhoto( ctx, sqlc.CreatePhotoParams {
			Owner: sql.NullInt64 { 1, true }, // TODO
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

type MapboxGeocodingFeature struct {
	Address string
	Latitude float32
	Longitude float32
}

type MapboxGeocodingResponse struct {
	Features []MapboxGeocodingFeature `json:"features"`
}

func ( feature *MapboxGeocodingFeature ) UnmarshalJSON( b []byte ) error {
	var f interface{}
	json.Unmarshal( b, &f )

	m := f.( map[string]interface{} )

	properties := m[ "properties" ].( map[string]interface{} )
	geometry := m[ "geometry" ].( map[string]interface{} )
	coordinates := geometry[ "coordinates" ].( []interface{} )

	feature.Address = properties[ "full_address" ].( string )
	feature.Latitude = float32( coordinates[ 0 ].( float64 ) )
	feature.Longitude = float32( coordinates[ 1 ].( float64 ) )

	return nil
}

func geocode() {
	resp := try1( http.Get( "https://api.mapbox.com/search/geocode/v6/forward?q=Los%20Angeles&access_token=pk.eyJ1IjoibWlrZWpzYXZhZ2UiLCJhIjoiY2x6bGZ0ajI0MDI2YTJrcG5tc2tmazZ1ZCJ9.vMTIB8J0J9fAiI2IrNrc5w" ) )
	body := try1( io.ReadAll( resp.Body ) )
	fmt.Printf( "%s\n", body )
	var decoded MapboxGeocodingResponse
	try( json.Unmarshal( []byte( body ), &decoded ) )

	for _, feature := range decoded.Features {
		fmt.Printf( "%s %f,%f\n", feature.Address, feature.Latitude, feature.Longitude )
	}
}

func exeChecksum() string {
	path := must1( os.Executable() )
	f := must1( os.Open( path ) )
	defer f.Close()

	hasher := fnv.New64a()
	_ = must1( io.Copy( hasher, f ) )

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

func pathValueAsset( r *http.Request ) ( string, []byte, error ) {
	sha256_str := r.PathValue( "asset" )
	sha256, err := hex.DecodeString( sha256_str )
	if err != nil || len( sha256 ) != 32 {
		return "", sha256, errors.New( "asset not a sha256" )
	}
	return sha256_str, sha256, nil
}

func serveAsset( w http.ResponseWriter, r *http.Request, sha256 string, asset_type string, original_filename string ) {
	ext := ".jpg"
	if asset_type == "heic" {
		accepts := strings.Split( r.Header.Get( "Accept" ), "," )
		ext = sel( slices.Contains( accepts, "image/heic" ), ".heic", ".heic.jpg" )
	}

	filename := "assets/" + sha256 + ext
	f := try1( os.Open( filename ) )
	defer f.Close()

	cacheControlImmutable( w )
	w.Header().Set( "Content-Disposition", fmt.Sprintf( "inline; filename=\"%s%s\"", original_filename, sel( ext == ".heic.jpg", ".jpg", "" ) ) )
	w.Header().Set( "Content-Type", sel( ext == ".heic", "image/heic", "image/jpeg" ) )

	_ = try1( io.Copy( w, f ) )
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
		Owner: sql.NullInt64 { user.ID, true },
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
		io.WriteString( w, "Name can't be blank" )
		return
	}
	if url == "" {
		io.WriteString( w, "URL can't be blank" )
		return
	}

	err = queries.SetAlbumSettings( r.Context(), sqlc.SetAlbumSettingsParams {
		Name: r.PostFormValue( "name" ),
		UrlSlug: r.PostFormValue( "url" ),
		ID: album_id,
		Owner: user.ID,
	} )
	if err != nil {
		io.WriteString( w, err.Error() )
		return
	}

	w.Header().Set( "HX-Redirect", "/" + r.PostFormValue( "url" ) )
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

func downloadAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	pathAlbumHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, album sqlc.GetAlbumByURLRow ) {
		query := r.URL.Query()
		download_everything := query.Get( "variants" ) == "everything"
		download_raws := query.Get( "variants" ) != "key_only"

		rows := try1( queries.GetAlbumAssets( r.Context(), sqlc.GetAlbumAssetsParams {
			ID: album.ID,
			Column2: !download_everything,
			Column3: !download_raws,
		} ) )

		files := make( []ZipFile, len( rows ) )
		for i, row := range rows {
			files[ i ] = ZipFile {
				Sha256: row.Asset,
				Type: row.Type,
			}
		}

		serveZip( album.Name, files, true, w )
	} )
}

type Photo struct {
	ID int64 `json:"id"`
	Asset string `json:"asset"`
	Thumbhash string `json:"thumbhash"`
}

func viewLibrary( w http.ResponseWriter, r *http.Request, user User ) {
	photos := []Photo { }
	for _, photo := range must1( queries.GetUserPhotos( r.Context(), sql.NullInt64 { user.ID, true } ) ) {
		photos = append( photos, Photo {
			ID: photo.ID,
			Asset: hex.EncodeToString( photo.Sha256 ),
			Thumbhash: base64.StdEncoding.EncodeToString( photo.Thumbhash ),
		} )
	}

	body := photogrid( photos, "/Special:asset/", "/Special:thumbnail/" )
	try( baseWithSidebar( user, checksum, r.URL.Path, "Library", body ).Render( r.Context(), w ) )
}

func viewAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	pathAlbumHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, album sqlc.GetAlbumByURLRow ) {
		photos := []Photo { }
		for _, photo := range try1( queries.GetAlbumPhotos( r.Context(), album.ID ) ) {
			photos = append( photos, Photo {
				ID: photo.ID,
				Asset: hex.EncodeToString( photo.Sha256 ),
				Thumbhash: base64.StdEncoding.EncodeToString( photo.Thumbhash ),
			} )
		}

		album_templ := ownedAlbumTemplate( album, photos )
		try( baseWithSidebar( user, checksum, r.URL.Path, album.Name, album_templ ).Render( r.Context(), w ) )
	} )
}

func uploadToLibrary( w http.ResponseWriter, r *http.Request, user User ) {
	assets := make( []AddedAsset, len( r.MultipartForm.File[ "assets" ] ) )

	for i, header := range r.MultipartForm.File[ "assets" ] {
		f := try1( header.Open() )
		defer f.Close()
		assets[ i ] = try1( addAsset( r.Context(), try1( io.ReadAll( f ) ), header.Filename ) )
	}

	var photo_id sql.Null[ int64 ]
	for _, asset := range assets {
		photos := try1( queries.GetAssetPhotos( r.Context(), sqlc.GetAssetPhotosParams {
			AssetID: asset.Sha256[:],
			Owner: sql.NullInt64 { user.ID, true },
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
			Owner: sql.NullInt64 { user.ID, true },
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

func uploadToAlbum( w http.ResponseWriter, r *http.Request, user User ) {
	pathAlbumHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, album sqlc.GetAlbumByURLRow ) {
		assets := make( []AddedAsset, len( r.MultipartForm.File[ "assets" ] ) )

		for i, header := range r.MultipartForm.File[ "assets" ] {
			f := try1( header.Open() )
			defer f.Close()
			assets[ i ] = try1( addAsset( r.Context(), try1( io.ReadAll( f ) ), header.Filename ) )
		}

		var photo_id sql.Null[ int64 ]
		for _, asset := range assets {
			photos := try1( queries.GetAssetPhotos( r.Context(), sqlc.GetAssetPhotosParams {
				AssetID: asset.Sha256[:],
				Owner: sql.NullInt64 { user.ID, true },
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
				Owner: sql.NullInt64 { user.ID, true },
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
	} )
}

func uploadToPhoto( w http.ResponseWriter, r *http.Request, user User ) {
	pathPhotoHandler( w, r, user, func( w http.ResponseWriter, r *http.Request, user User, photo_id int64 ) {
		assets := make( []AddedAsset, len( r.MultipartForm.File[ "assets" ] ) )

		for i, header := range r.MultipartForm.File[ "assets" ] {
			f := try1( header.Open() )
			defer f.Close()
			assets[ i ] = try1( addAsset( r.Context(), try1( io.ReadAll( f ) ), header.Filename ) )
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

func viewAlbumAsGuest( w http.ResponseWriter, r *http.Request ) {
	album := queryOptional( queries.GetAlbumByURL( r.Context(), r.PathValue( "album" ) ) )

	if !album.Valid {
		httpError( w, http.StatusNotFound )
		return
	}

	secret := r.PathValue( "secret" )
	if secret != album.V.ReadonlySecret && secret != album.V.ReadwriteSecret {
		httpError( w, http.StatusForbidden )
		return
	}

	photos := []Photo { }
	for _, photo := range try1( queries.GetAlbumPhotos( r.Context(), album.V.ID ) ) {
		photos = append( photos, Photo {
			ID: photo.ID,
			Asset: hex.EncodeToString( photo.Sha256 ),
			Thumbhash: base64.StdEncoding.EncodeToString( photo.Thumbhash ),
		} )
	}

	album_templ := guestAlbumTemplate( album.V, photos, secret == album.V.ReadwriteSecret )
	try( guestBase( checksum, album.V.Name, album_templ ).Render( r.Context(), w ) )
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

func exifOrientation( exif exif2.Exif ) ( meta.Orientation, error ) {
	if exif.Orientation < meta.OrientationHorizontal || exif.Orientation > meta.OrientationRotate270 {
		return 0, errors.New( "EXIF orientation out of range" )
	}

	return exif.Orientation, nil
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

	scale := math.Min( 1, thumbnail_size / float64( min( image.Rect.Dx(), image.Rect.Dy() ) ) )
	thumbnail := stb.StbResize( image, int( float64( image.Rect.Dx() ) * scale ), int( float64( image.Rect.Dy() ) * scale ) )
	thumbnail_jpg := must1( stb.StbToJpg( thumbnail, 75 ) )

	return thumbnail_jpg, thumbhash.EncodeImage( thumbnail )
}

func saveAsset( data []byte, filename string ) error {
	return os.WriteFile( "assets/" + filename, data, 0644 )
}

type AddedAsset struct {
	Sha256 [32]byte
	Date sql.NullInt64
	Latitude sql.NullFloat64
	Longitude sql.NullFloat64
}

func addAsset( ctx context.Context, data []byte, filename string ) ( AddedAsset, error ) {
	sha256 := sha256.Sum256( data )

	var date sql.NullInt64
	var latitude sql.NullFloat64
	var longitude sql.NullFloat64
	orientation := meta.OrientationHorizontal

	exif, err := imagemeta.Decode( bytes.NewReader( data ) )
	if err == nil {
		if exif.Orientation >= meta.OrientationHorizontal && exif.Orientation <= meta.OrientationMirrorHorizontalRotate90 {
			orientation = exif.Orientation
		}

		if !exif.CreateDate().IsZero() {
			date = sql.NullInt64 { exif.CreateDate().Unix(), true }
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

	is_heic := strings.ToLower( filepath.Ext( filename ) ) == ".heic"
	ext := sel( is_heic, ".heic", ".jpg" )

	var decoded *image.RGBA
	if is_heic {
		ycbcr, err := goheif.Decode( bytes.NewReader( data ) )
		fmt.Printf( "\tdecoded HEIC %dms\n", time.Now().Sub( before ).Milliseconds() )
		if err != nil {
			return AddedAsset { }, err
		}

		decoded = image.NewRGBA( ycbcr.Bounds() )
		draw.Draw( decoded, decoded.Bounds(), ycbcr, ycbcr.Bounds().Min, draw.Src )
		fmt.Printf( "\tyCbCr -> RGBA %dms\n", time.Now().Sub( before ).Milliseconds() )
	} else {
		decoded, err = stb.StbLoad( data )
		if err != nil {
			return AddedAsset { }, err
		}
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

	fmt.Printf( "\tEXIF decoded %dms\n", time.Now().Sub( before ).Milliseconds() )

	reoriented := reorient( decoded, orientation )
	fmt.Printf( "\treoriented %dms\n", time.Now().Sub( before ).Milliseconds() )

	hex_sha256 := hex.EncodeToString( sha256[:] )
	err = saveAsset( data, hex_sha256 + ext )
	if err != nil {
		return AddedAsset { }, err
	}
	if is_heic {
		jpeg := must1( stb.StbToJpg( reoriented, 95 ) )
		fmt.Printf( "\theic -> jpeg %dms %d -> %d\n", time.Now().Sub( before ).Milliseconds(), len( data ), len( jpeg ) )
		err = saveAsset( jpeg, hex_sha256 + ".heic.jpg" )
		if err != nil {
			return AddedAsset { }, err
		}
	}
	fmt.Printf( "\tsave assets %dms\n", time.Now().Sub( before ).Milliseconds() )

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

	fmt.Printf( "\tdone %dms\n", time.Now().Sub( before ).Milliseconds() )

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
		Owner: sql.NullInt64 { user, true },
	} )

	if len( photos ) == 0 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		qtx := queries.WithTx( tx )

		photo_id, err := qtx.CreatePhoto( ctx, sqlc.CreatePhotoParams {
			Owner: sql.NullInt64 { user, true },
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

func pathAlbumHandler( w http.ResponseWriter, r *http.Request, user User, handler func( http.ResponseWriter, *http.Request, User, sqlc.GetAlbumByURLRow ) ) {
	album := queryOptional( queries.GetAlbumByURL( r.Context(), r.PathValue( "album" ) ) )
	if !album.Valid {
		httpError( w, http.StatusNotFound )
		return
	}

	if album.V.Owner != user.ID && album.V.Shared == 0 {
		httpError( w, http.StatusForbidden )
		return
	}

	handler( w, r, user, album.V )
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

func loginForm( w http.ResponseWriter, r *http.Request ) {
	try( loginFormTemplate( checksum ).Render( r.Context(), w ) )
}

// TODO: for some reason these don't work in safari
func setAuthCookies( w http.ResponseWriter, username string, auth string ) {
	expiration := -1
	if auth != "" {
		expiration = int( ( 365 * 24 * time.Hour ).Seconds() )
	}

	cookie := http.Cookie {
		Name: "username",
		Value: username,
		Path: "/",
		MaxAge: expiration,
		// Secure: true, TODO: look at the Forwarded header? Forwarded: for=192.0.2.60;proto=http;by=203.0.113.43
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	http.SetCookie( w, &cookie )

	cookie.Name = "auth"
	cookie.Value = auth

	http.SetCookie( w, &cookie )
}

func authenticate( w http.ResponseWriter, r *http.Request ) {
	form_username := norm.NFKC.String( r.PostFormValue( "username" ) )
	form_password := norm.NFKC.String( r.PostFormValue( "password" ) )

	user := queryOptional( queries.GetUserAuthDetails( r.Context(), form_username ) )
	if !user.Valid {
		http.Error( w, "Incorrect username", http.StatusOK )
		return
	}

	if form_password != user.V.Password {
		http.Error( w, "Incorrect password", http.StatusOK )
		return
	}

	setAuthCookies( w, form_username, user.V.Cookie )
	w.Header().Set( "HX-Refresh", "true" )
}

func logout( w http.ResponseWriter, r *http.Request ) {
	setAuthCookies( w, "", "" )
	http.Redirect( w, r, "/", http.StatusSeeOther )
}

type ZipFile struct {
	Sha256 []byte
	Type string
}

func serveZip( filename string, assets []ZipFile, heic_as_jpeg bool, w http.ResponseWriter ) {
	// TODO: compute Content-Length
	w.Header().Set( "Content-Disposition", fmt.Sprintf( "attachment; filename=\"%s.zip\"", filename ) )
	w.Header().Set( "Content-Type", "application/zip" )

	zip := zip.NewWriter( w )

	for _, asset := range assets {
		disk_extension := asset.Type
		zip_extension := asset.Type
		if heic_as_jpeg && asset.Type == "heic" {
			disk_extension = "heic.jpg"
			zip_extension = "jpg"
		}

		filename := hex.EncodeToString( asset.Sha256 )
		a := try1( os.Open( "assets/" + filename + "." + disk_extension ) )
		defer a.Close()

		z := try1( zip.Create( filename + "." + zip_extension ) )
		_ = try1( io.Copy( z, a ) )
	}

	try( zip.Close() )
}

func requireAuth( handler func( http.ResponseWriter, *http.Request, User ) ) func( http.ResponseWriter, *http.Request ) {
	return func( w http.ResponseWriter, r *http.Request ) {
		authed := false

		username, err_username := r.Cookie( "username" )
		auth, err_auth := r.Cookie( "auth" )

		var user User

		if err_username == nil && err_auth == nil {
			row := queryOptional( queries.GetUserAuthDetails( r.Context(), username.Value ) )
			if row.Valid {
				// subtle.WithDataIndependentTiming( func() { // needs very very new go
					if subtle.ConstantTimeCompare( []byte( auth.Value ), []byte( row.V.Cookie ) ) == 1 {
						authed = true
						user = User { row.V.ID, username.Value }
					}
				// } )
			}
		}

		if !authed {
			if r.Method == "GET" {
				w.WriteHeader( http.StatusUnauthorized )
				loginForm( w, r )
			} else {
				httpError( w, http.StatusForbidden )
			}
			return
		}

		setAuthCookies( w, user.Username, auth.Value )
		handler( w, r, user )
	}
}

func serveString( content string ) func( http.ResponseWriter, *http.Request ) {
	return func( w http.ResponseWriter, r *http.Request ) {
		cacheControlImmutable( w )
		_ = try1( w.Write( []byte( content ) ) )
	}
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

func startHttpServer( addr string, routes []Route ) *http.Server {
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
			httpError( w, http.StatusNotFound )
		}
	} )

	http_server := &http.Server{
		Addr: addr,
		Handler: mux,
	}

	go func() {
		err := http_server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatal( err )
		}
	}()

	return http_server
}

func showHelpAndQuit() {
	os.Exit( 1 )
}

func main() {
	runtime.GOMAXPROCS( 2 )

	checksum = exeChecksum()

	err := os.Mkdir( "assets", 0755 )
	if err != nil && !errors.Is( err, os.ErrExist ) {
		log.Fatalf( "Can't make assets dir: %v", err )
	}

	db_path := "file::memory:"
	if len( os.Args ) == 1 {
		if IsReleaseBuild {
			showHelpAndQuit()
		}

		fmt.Println( "Using in memory database. Nothing will be saved when you quit the server!" )
	} else {
		db_path = os.Args[ 1 ]
	}

	db = must1( sql.Open( "sqlite3", db_path + "?cache=shared" ) )
	defer db.Close()

	queries = sqlc.New( db )

	initDB()

	if must1( queries.AreThereAnyUsers( context.Background() ) ) == 0 {
		fmt.Printf( "You need to create a user by running \"%s create-user\" first!", os.Args[ 0 ] )
		os.Exit( 1 )
	}

	{
		minifier = minify.New()
		minifier.AddFunc( "js", minify_js.Minify )
		thumbhashjs = must1( minifier.String( "js", strings.ReplaceAll( thumbhashjs, "export function", "function" ) ) )
	}

	fs_watcher := initFSWatcher()
	defer fs_watcher.Close()

	private_http_server := startHttpServer( "0.0.0.0:5678", []Route {
		{ "GET",  "/Special:checksum", getChecksum },
		{ "GET",  "/Special:alpinejs-3.14.9.js", serveString( alpinejs ) },
		{ "GET",  "/Special:fuzzysort-3.1.0.js", serveString( fuzzysortjs ) },
		{ "GET",  "/Special:htmx-2.0.4.js", serveString( htmxjs ) },
		{ "GET",  "/Special:thumbhash-1.0.0.js", serveString( thumbhashjs ) },

		{ "POST", "/Special:authenticate", authenticate },
		{ "GET",  "/Special:logout", logout },

		{ "GET",  "/Special:asset/{asset}", requireAuth( getAsset ) },
		{ "GET",  "/Special:thumbnail/{asset}", requireAuth( getThumbnail ) },
		// { "GET",  "/Special:geocode", geocode },

		{ "POST", "/Special:albumSettings", requireAuth( updateAlbumSettings ) },
		{ "POST", "/Special:share", requireAuth( shareAlbum ) },

		{ "GET",  "/Special:download/{album}", requireAuth( downloadAlbum ) },
		// { "POST", "/Special:download", requireAuth( downloadPhotos ) },

		{ "GET",  "/", requireAuth( viewLibrary ) },
		{ "GET",  "/{album}", requireAuth( viewAlbum ) },

		{ "POST", "/", requireAuth( uploadToLibrary ) },
		{ "POST", "/{album}", requireAuth( uploadToAlbum ) },
		{ "POST", "/Special:uploadToPhoto", requireAuth( uploadToPhoto ) },
	} )

	guest_http_server := startHttpServer( "0.0.0.0:5679", []Route {
		{ "GET",  "/Special:checksum", getChecksum },
		{ "GET",  "/Special:alpinejs-3.14.9.js", serveString( alpinejs ) },
		{ "GET",  "/Special:fuzzysort-3.1.0.js", serveString( fuzzysortjs ) },
		{ "GET",  "/Special:htmx-2.0.4.js", serveString( htmxjs ) },
		{ "GET",  "/Special:thumbhash-1.0.0.js", serveString( thumbhashjs ) },

		{ "GET",  "/{album}/{secret}", viewAlbumAsGuest },
		{ "GET",  "/{album}/{secret}/asset/{asset}", getAssetAsGuest },
		{ "GET",  "/{album}/{secret}/thumbnail/{asset}", getThumbnailAsGuest },
		// { "POST", "/{album}/{secret}", uploadPhotosAsGuest ) },
	} )

	guest_url = "http://localhost:5679"
	fmt.Printf( "http://localhost:5678/ %s\n", guest_url )

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
}
