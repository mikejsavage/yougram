package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html/template"
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
	"strconv"
	"strings"
	"syscall"
	"time"

	"mikegram/sqlc"
	"mikegram/stb"

	"golang.org/x/text/unicode/norm"
	"github.com/adrium/goheif"
	"github.com/galdor/go-thumbhash"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
	// "github.com/evanoberholster/imagemeta" TODO
	"github.com/tdewolff/minify/v2"
	"github.com/fsnotify/fsnotify"
	minify_css "github.com/tdewolff/minify/v2/css"
	minify_html "github.com/tdewolff/minify/v2/html"
	minify_js "github.com/tdewolff/minify/v2/js"
	_ "modernc.org/sqlite"
)

var db *sql.DB
var queries *sqlc.Queries

//go:embed schema.sql
var db_schema string

//go:embed alpine-3.14.9.js
var alpinejs string
//go:embed htmx-2.0.4.js
var htmx string
//go:embed thumbhash.js
var thumbhashjs string

var minifier *minify.M

//go:embed *.html
var template_sources embed.FS
var templates *template.Template

var checksum string

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

func must2[ T1 any, T2 any ]( v1 T1, v2 T2, err error ) ( T1, T2 ) {
	mustImpl( err )
	return v1, v2
}

func try( err error ) {
	if err != nil {
		panic( err )
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

	must( queries.CreateUser( ctx, sqlc.CreateUserParams {
		Username: norm.NFKC.String( "mike" ),
		Password: norm.NFKC.String( "gg" ),
		Cookie: "123",
	} ) )

	must( queries.CreateAlbum( ctx, sqlc.CreateAlbumParams {
		Owner: 1,
		Name: "France 2024",
		UrlSlug: "france-2024",
		Shared: 1,
		ReadonlySecret: sql.NullString { "aaaaaaaa", true },
		ReadwriteSecret: sql.NullString { "bbbbbbbb", true },
	} ) )
	must( queries.CreateAlbum( ctx, sqlc.CreateAlbumParams {
		Owner: 1,
		Name: "Helsinki 2024",
		UrlSlug: "helsinki-2024",
		Shared: 0,
		ReadonlySecret: sql.NullString { "aaaaaaaa", true },
		ReadwriteSecret: sql.NullString { "bbbbbbbb", true },
		AutoassignStartDate: sql.NullInt64 { time.Date( 2024, time.January, 1, 0, 0, 0, 0, time.UTC ).Unix(), true },
		AutoassignEndDate: sql.NullInt64 { time.Date( 2024, time.December, 31, 0, 0, 0, 0, time.UTC ).Unix(), true },
		AutoassignLatitude: sql.NullFloat64 { 60.1699, true },
		AutoassignLongitude: sql.NullFloat64 { 24.9384, true },
		AutoassignRadius: sql.NullFloat64 { 50, true },
	} ) )

	must( addFileToAlbum( ctx, "DSCN0025.jpg", 2 ) )
	must( addFile( ctx, "776AE6EC-FBF4-4549-BD58-5C442DA2860D.JPG", sql.Null[ int64 ] { } ) )
	must( addFile( ctx, "IMG_2330.HEIC", sql.Null[ int64 ] { } ) )
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

func cors( w http.ResponseWriter ) {
	w.Header().Set( "Access-Control-Allow-Origin", "*" )
}

func httpError( w http.ResponseWriter, status int ) {
	http.Error( w, http.StatusText( status ), status )
}

func cacheControlImmutable( w http.ResponseWriter ) {
	w.Header().Set( "Cache-Control", "max-age=31536000, immutable" )
}

func getChecksum( w http.ResponseWriter, r *http.Request, route []string ) {
	io.WriteString( w, checksum )
}

// TODO: func canViewImage( user, route[ 0 ] ) ( bool, httpstatus )

func getImage( w http.ResponseWriter, r *http.Request, route []string, user User ) {
	image_id, err := strconv.ParseInt( route[ 0 ], 10, 64 )
	if err != nil {
		httpError( w, http.StatusBadRequest )
		return
	}

	photo, err := queries.GetPhoto( r.Context(), sqlc.GetPhotoParams { image_id, image_id } )
	if err != nil {
		if errors.Is( err, sql.ErrNoRows ) {
			httpError( w, http.StatusNotFound )
			return
		}
		log.Fatal( err )
	}

	// TODO: serve heif if possible
	filename := "assets/" + hex.EncodeToString( photo.Sha256 ) + sel( photo.Type == "heif", ".heif.jpg", ".jpg" )
	f := try1( os.Open( filename ) )
	defer f.Close()

	cacheControlImmutable( w )
	w.Header().Set( "Content-Disposition", fmt.Sprintf( "inline; filename=\"%s\"", photo.OriginalFilename ) )
	w.Header().Set( "Content-Type", "image/jpeg" )

	_ = try1( io.Copy( w, f ) )
}

func getThumbnail( w http.ResponseWriter, r *http.Request, route []string, user User ) {
	image_id, err := strconv.ParseInt( route[ 0 ], 10, 64 )
	if err != nil {
		httpError( w, http.StatusBadRequest )
		return
	}

	thumbnail, err := queries.GetThumbnail( r.Context(), image_id )
	if err != nil {
		if errors.Is( err, sql.ErrNoRows ) {
			httpError( w, http.StatusNotFound )
			return
		}
		log.Fatal( err )
	}

	cacheControlImmutable( w )
	w.Header().Set( "Content-Disposition", fmt.Sprintf( "inline; filename=\"thumb_%s.jpg\"", route[ 0 ] ) )
	w.Header().Set( "Content-Type", "image/jpeg" )
	_ = try1( w.Write( thumbnail ) )
}

type User struct {
	ID int64
	Username string
}

type AlbumMetadata struct {
	ID int64
	Name string
	Writeable bool
}

func findAlbum( ctx context.Context, user User, route []string ) ( AlbumMetadata, int ) {
	album, err := queries.GetAlbumByURL( ctx, route[ 0 ] )
	if err != nil {
		if errors.Is( err, sql.ErrNoRows ) {
			return AlbumMetadata { }, http.StatusNotFound
		}
		panic( err )
	}

	if album.Owner != user.ID && album.Shared == 0 {
		return AlbumMetadata { }, http.StatusForbidden
	}

	return AlbumMetadata {
		ID: album.ID,
		Name: album.Name,
		Writeable: true,
	}, http.StatusOK
}

func viewLibrary( w http.ResponseWriter, r *http.Request, route []string, user User ) {
}

func viewAlbum( w http.ResponseWriter, r *http.Request, route []string, user User ) {
	album, http_status := findAlbum( r.Context(), user, route )
	if http_status != http.StatusOK {
		httpError( w, http_status )
		return
	}

	type Photo struct {
		ID int64
		Thumbhash string
	}

	var photos []Photo
	for _, photo := range must1( queries.GetAlbumPhotos( r.Context(), album.ID ) ) {
		photos = append( photos, Photo {
			ID: photo.ID,
			Thumbhash: base64.StdEncoding.EncodeToString( photo.Thumbhash ),
		} )
	}

	var album_html bytes.Buffer
	{
		context := struct {
			Title string
			AlbumURL string
			Photos []Photo
			ShowUpload bool
		}{
			Title: album.Name,
			AlbumURL: route[ 0 ],
			Photos: photos,
			ShowUpload: album.Writeable,
		}
		try( templates.ExecuteTemplate( &album_html, "album.html", context ) )
	}

	var page bytes.Buffer
	{
		context := struct {
			Title string
			Checksum string
			AlpineJS template.JS
			ThumbhashJS template.HTML
			Body template.HTML
		}{
			Title: album.Name,
			Checksum: checksum,
			AlpineJS: template.JS( alpinejs + htmx ),
			ThumbhashJS: template.HTML( thumbhashjs ),
			Body: template.HTML( album_html.String() ),
		}
		try( templates.ExecuteTemplate( &page, "base.html", context ) )
	}

	minified := try1( minifier.String( "html", page.String() ) )
	_ = try1( w.Write( []byte( minified ) ) )
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

type ExifOrientation int

const (
	ExifOrientation_Identity ExifOrientation = 1
	ExifOrientation_FlipHorizontal ExifOrientation = 2
	ExifOrientation_Rotate180 ExifOrientation = 3
	ExifOrientation_FlipVertical ExifOrientation = 4
	ExifOrientation_Transpose ExifOrientation = 5
	ExifOrientation_Rotate270 ExifOrientation = 6
	ExifOrientation_OppositeTranspose ExifOrientation = 7
	ExifOrientation_Rotate90 ExifOrientation = 8
)

func exifOrientation( data *exif.Exif ) ( ExifOrientation, error ) {
	tag, err := data.Get( exif.Orientation )
	if err != nil {
		return ExifOrientation_Identity, err
	}

	if tag.Format() != tiff.IntVal || tag.Count != 1 {
		return ExifOrientation_Identity, errors.New( "EXIF Orientation not an int" )
	}

	orientation, err := tag.Int( 0 )
	if err != nil {
		return ExifOrientation_Identity, err
	}

	if ExifOrientation( orientation ) < ExifOrientation_Identity || ExifOrientation( orientation ) > ExifOrientation_Rotate90 {
		return ExifOrientation_Identity, errors.New( "EXIF orientation out of range" )
	}

	return ExifOrientation( orientation ), nil
}

func reorient( img *image.RGBA, orientation ExifOrientation ) *image.RGBA {
	if orientation == ExifOrientation_Identity {
		return img
	}

	type Walker struct {
		Origin image.Point
		Dx image.Point
		Dy image.Point
	}

	var walkers = map[ ExifOrientation ] Walker {
		ExifOrientation_FlipHorizontal:    { image.Point { -1, +1 }, image.Point { -1, +0 }, image.Point { +0, +1 } },
		ExifOrientation_Rotate180:         { image.Point { -1, -1 }, image.Point { -1, +0 }, image.Point { +0, -1 } },
		ExifOrientation_FlipVertical:      { image.Point { +1, -1 }, image.Point { +1, +0 }, image.Point { +0, -1 } },
		ExifOrientation_Transpose:         { image.Point { +1, +1 }, image.Point { +0, +1 }, image.Point { +1, +0 } },
		ExifOrientation_Rotate270:         { image.Point { -1, +1 }, image.Point { +0, +1 }, image.Point { -1, +0 } },
		ExifOrientation_OppositeTranspose: { image.Point { -1, -1 }, image.Point { +0, -1 }, image.Point { -1, +0 } },
		ExifOrientation_Rotate90:          { image.Point { +1, -1 }, image.Point { +0, -1 }, image.Point { +1, +0 } },
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

	scale := thumbnail_size / float32( min( image.Rect.Dx(), image.Rect.Dy() ) )
	thumbnail := stb.StbResize( image, int( float32( image.Rect.Dx() ) * scale ), int( float32( image.Rect.Dy() ) * scale ) )
	thumbnail_jpg := must1( stb.StbToJpg( thumbnail, 75 ) )

	return thumbnail_jpg, thumbhash.EncodeImage( thumbnail )
}

func saveAsset( data []byte, filename string ) error {
	return os.WriteFile( "assets/" + filename, data, 0644 )
}

type AddedAsset struct {
	ID int64
	Image *image.RGBA
	Date sql.NullInt64
	Latitude sql.NullFloat64
	Longitude sql.NullFloat64
}

func addAsset( ctx context.Context, data []byte, album_id sql.Null[ int64 ], filename string ) ( AddedAsset, error ) {
	before := time.Now()
	fmt.Printf( "addAsset( %s )\n", filename )

	is_heif := strings.ToLower( filepath.Ext( filename ) ) == ".heic"
	ext := sel( is_heif, ".heic", ".jpg" )

	var decoded *image.RGBA
	var err error
	if is_heif {
		ycbcr, err := goheif.Decode( bytes.NewReader( data ) )
		fmt.Printf( "\tdecoded HEIF %dms\n", time.Now().Sub( before ).Milliseconds() )
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

	sha256 := sha256.Sum256( data )

	var date sql.NullInt64
	var latitude sql.NullFloat64
	var longitude sql.NullFloat64
	orientation := ExifOrientation_Identity

	var exif_src []byte
	if is_heif {
		exif_src, _ = goheif.ExtractExif( bytes.NewReader( data ) )
	} else {
		exif_src = data
	}

	exif, err := exif.Decode( bytes.NewReader( exif_src ) )
	if err == nil {
		maybe_orientation, err := exifOrientation( exif )
		if err == nil {
			orientation = maybe_orientation
		}

		maybe_date, err := exif.DateTime()
		if err == nil {
			date = sql.NullInt64 { maybe_date.Unix(), true }
		}

		maybe_latitude, maybe_longitude, err := exif.LatLong()
		if err == nil {
			latitude = sql.NullFloat64 { maybe_latitude, true }
			longitude = sql.NullFloat64 { maybe_longitude, true }
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
	if is_heif {
		jpeg := must1( stb.StbToJpg( reoriented, 95 ) )
		fmt.Printf( "\theic -> jpeg %dms %d -> %d\n", time.Now().Sub( before ).Milliseconds(), len( data ), len( jpeg ) )
		err = saveAsset( jpeg, hex_sha256 + ".heif.jpg" )
		if err != nil {
			return AddedAsset { }, err
		}
	}
	fmt.Printf( "\tsave assets %dms\n", time.Now().Sub( before ).Milliseconds() )

	asset_id, err := queries.CreateAsset( ctx, sqlc.CreateAssetParams {
		Sha256: sha256[:],
		CreatedAt: time.Now().Unix(),
		OriginalFilename: filename,
		Type: sel( is_heif, "heif", "jpeg" ),
		DateTaken: date,
		Latitude: latitude,
		Longitude: longitude,
	} )

	fmt.Printf( "\tdone %dms\n", time.Now().Sub( before ).Milliseconds() )

	return AddedAsset { asset_id, reoriented, date, latitude, longitude }, err
}

func addFile( ctx context.Context, path string, album_id sql.Null[ int64 ] ) error {
	f := must1( os.Open( path ) )
	defer f.Close()
	img := must1( io.ReadAll( f ) )
	asset, err := addAsset( ctx, img, album_id, path )
	if err != nil {
		return err
	}

	photos, err := queries.GetAssetPhotos( ctx, sqlc.GetAssetPhotosParams {
		AssetID: asset.ID,
		Owner: sql.NullInt64 { 1, true }, // TODO
	} )

	if len( photos ) == 0 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		qtx := queries.WithTx( tx )

		thumbnail, thumbhash := generateThumbnail( asset.Image )

		photo_id, err := qtx.CreatePhoto( ctx, sqlc.CreatePhotoParams {
			Owner: sql.NullInt64 { 1, true }, // TODO
			CreatedAt: time.Now().Unix(),
			PrimaryAsset: asset.ID,
			Thumbnail: thumbnail,
			Thumbhash: thumbhash,
			DateTaken: asset.Date,
			Latitude: asset.Latitude,
			Longitude: asset.Longitude,
		} )
		if err != nil {
			return err
		}

		err = qtx.AddAssetToPhoto( ctx, sqlc.AddAssetToPhotoParams {
			PhotoID: photo_id,
			AssetID: asset.ID,
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

func addFileToAlbum( ctx context.Context, path string, album_id int64 ) error {
	return addFile( ctx, path, sql.Null[ int64 ] { album_id, true } )
}

func uploadPhotos( w http.ResponseWriter, r *http.Request, route []string, user User ) {
	album, http_status := findAlbum( r.Context(), user, route )
	if http_status != http.StatusOK || !album.Writeable {
		httpError( w, sel( http_status == http.StatusOK, http.StatusForbidden, http_status ) )
		return
	}

	const megabyte = 1000 * 1000
	try( r.ParseMultipartForm( 10 * megabyte ) )

	for _, header := range r.MultipartForm.File[ "photos" ] {
		f := try1( header.Open() )
		defer f.Close()

		contents := try1( io.ReadAll( f ) )

		fmt.Printf( "%s -> %d %d\n", header.Filename, len( contents ), album.ID )
	}

	http.Redirect( w, r, r.URL.Path, http.StatusSeeOther )
}

func loginForm( w http.ResponseWriter, r *http.Request ) {
	context := struct {
		Checksum string
	}{
		Checksum: checksum,
	}

	var page bytes.Buffer
	try( templates.ExecuteTemplate( &page, "login.html", context ) )

	minified := try1( minifier.String( "html", page.String() ) )
	_ = try1( w.Write( []byte( minified ) ) )
}

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
		Secure: true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	http.SetCookie( w, &cookie )

	cookie.Name = "auth"
	cookie.Value = auth

	http.SetCookie( w, &cookie )
}

func authenticate( w http.ResponseWriter, r *http.Request, route []string ) {
	form_username := norm.NFKC.String( r.PostFormValue( "username" ) )
	form_password := norm.NFKC.String( r.PostFormValue( "password" ) )
	row := db.QueryRow( "SELECT password, cookie FROM users WHERE username = $1", form_username )

	var password string
	var cookie string
	err := row.Scan( &password, &cookie )
	if err != nil {
		if errors.Is( err, sql.ErrNoRows ) {
			http.Error( w, "Incorrect username", http.StatusOK )
			return
		}
		log.Fatal( err )
	}

	if form_password != password {
		http.Error( w, "Incorrect password", http.StatusOK )
		return
	}

	setAuthCookies( w, form_username, cookie )

	w.Header().Set( "HX-Refresh", "true" )
}

func logout( w http.ResponseWriter, r *http.Request, route []string, user User ) {
	setAuthCookies( w, "", "" )
	http.Redirect( w, r, "/", http.StatusSeeOther )
}

type Route struct {
	Regex *regexp.Regexp
	Method string
	Handler func( http.ResponseWriter, *http.Request, []string )
}

func requireAuth( handler func( w http.ResponseWriter, r *http.Request, route []string, user User ) ) func( w http.ResponseWriter, r *http.Request, route []string ) {
	return func( w http.ResponseWriter, r *http.Request, route []string ) {
		authed := false

		username, err_username := r.Cookie( "username" )
		auth, err_auth := r.Cookie( "auth" )

		var user User

		if err_username == nil && err_auth == nil {
			row, err := queries.GetUserAuthDetails( r.Context(), username.Value )
			if err == nil {
				// subtle.WithDataIndependentTiming( func() { // needs very very new go
					if subtle.ConstantTimeCompare( []byte( auth.Value ), []byte( row.Cookie ) ) == 1 {
						authed = true
						user = User { row.ID, username.Value }
					}
				// } )
			} else if !errors.Is( err, sql.ErrNoRows ) {
				log.Fatal( err )
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
		handler( w, r, route, user )
	}
}

func startHttpServer( addr string, routes []Route ) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc( "/", func( w http.ResponseWriter, r *http.Request ) {
		defer func() {
			if r := recover(); r != nil {
				log.Print( r )
				httpError( w, http.StatusInternalServerError )
				return
			}
		}()

		is405 := false

		for _, route := range routes {
			if matches := route.Regex.FindStringSubmatch( r.URL.Path ); len( matches ) > 0 {
				if r.Method == route.Method {
					route.Handler( w, r, matches[ 1: ] )
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

func main() {
	runtime.GOMAXPROCS( 2 )

	checksum = exeChecksum()

	err := os.Mkdir( "assets", 0755 )
	if err != nil && !errors.Is( err, os.ErrExist ) {
		log.Fatalf( "Can't make assets dir: %v", err )
	}

	{
		path := "file::memory:?cache=shared"
		if len( os.Args ) == 2 {
			path = os.Args[ 1 ]
		} else {
			fmt.Println( "Using in memory database. Nothing will be saved when you quit the server!" )
		}
		db = must1( sql.Open( "sqlite", path ) )
	}
	defer db.Close()

	queries = sqlc.New( db )

	initDB()

	{
		minifier = minify.New()
		minifier.AddFunc( "css", minify_css.Minify )
		minifier.AddFunc( "html", minify_html.Minify )
		minifier.AddFunc( "js", minify_js.Minify )

		thumbhashjs = "<script>" + must1( minifier.String( "js", strings.ReplaceAll( thumbhashjs, "export function", "function" ) ) ) + "</script>"

		css_finder := regexp.MustCompile( "(?s)<style>.*?</style>" )
		minifyStyleTag := func( css string ) string {
			trimmed := strings.TrimPrefix( strings.TrimSuffix( css, "</style>" ), "<style>" )
			return "<style>" + must1( minifier.String( "css", trimmed ) ) + "</style>"
		}

		script_finder := regexp.MustCompile( "(?s)<script>.*?</script>" )
		minifyScriptTag := func( js string ) string {
			trimmed := strings.TrimPrefix( strings.TrimSuffix( js, "</script>" ), "<script>" )
			return "<script>" + must1( minifier.String( "js", trimmed ) ) + "</script>"
		}

		minifyTemplate := func( filename string ) string {
			f := must1( template_sources.Open( filename ) )
			defer f.Close()

			minified := string( must1( io.ReadAll( f ) ) )
			minified = css_finder.ReplaceAllStringFunc( minified, minifyStyleTag )
			minified = script_finder.ReplaceAllStringFunc( minified, minifyScriptTag )

			return minified
		}

		template_funcs := template.FuncMap{
			"add": func( a int, b int ) int { return a + b },
		}
		templates = template.New( "dummy" ).Funcs( template_funcs )
		for _, f := range must1( template_sources.ReadDir( "." ) ) {
			templates.New( f.Name() ).Parse( minifyTemplate( f.Name() ) )
		}
	}

	fs_watcher := initFSWatcher()
	defer fs_watcher.Close()

	private_http_server := startHttpServer( "0.0.0.0:5678", []Route {
		{ regexp.MustCompile( "^/Special:authenticate$" ), "POST", authenticate },
		{ regexp.MustCompile( "^/Special:checksum$" ), "GET", getChecksum },
		{ regexp.MustCompile( "^/Special:image/([^/]+)$" ), "GET", requireAuth( getImage ) },
		{ regexp.MustCompile( "^/Special:logout$" ), "GET", requireAuth( logout ) },
		{ regexp.MustCompile( "^/Special:thumbnail/([^/]+)$" ), "GET", requireAuth( getThumbnail ) },
		// { regexp.MustCompile( "^/Special:geocode$" ), "GET", geocode },

		{ regexp.MustCompile( "^/$" ), "GET", requireAuth( viewLibrary ) },
		{ regexp.MustCompile( "^/([^:/]+)$" ), "GET", requireAuth( viewAlbum ) },
		{ regexp.MustCompile( "^/([^:/]+)/([^/]+)$" ), "GET", requireAuth( viewAlbum ) },
		{ regexp.MustCompile( "^/([^:/]+)/([^/]+)$" ), "POST", requireAuth( uploadPhotos ) },
	} )

	guest_http_server := startHttpServer( "0.0.0.0:5679", []Route {
		// { regexp.MustCompile( "^/Special:image/([^/]+)$" ), "GET", getImage },
		// { regexp.MustCompile( "^/Special:thumbnail/([^/]+)$" ), "GET", getThumbnail },
		// { regexp.MustCompile( "^/([^:/]+)$" ), "GET", viewAlbum },
		// { regexp.MustCompile( "^/([^:/]+)/([^/]+)$" ), "GET", viewAlbum },
	} )

	fmt.Printf( "http://localhost:5678/\n" )

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
