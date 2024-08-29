package main

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/galdor/go-thumbhash"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
	"github.com/tdewolff/minify/v2"
	minify_css "github.com/tdewolff/minify/v2/css"
	minify_js "github.com/tdewolff/minify/v2/js"
)

var db *sql.DB

//go:embed *.html
var template_sources embed.FS
var templates *template.Template

//go:embed alpine-3.14.1.js
var alpinejs string
//go:embed thumbhash.js
var thumbhashjs string

var checksum string

func sel[ T any ]( p bool, t T, f T ) T {
	if p {
		return t
	}
	return f
}

func must( err error ) {
	if err != nil {
		log.Fatal( err )
	}
}

func must1[ T1 any ]( v1 T1, err error ) T1 {
	must( err )
	return v1
}

func must2[ T1 any, T2 any ]( v1 T1, v2 T2, err error ) ( T1, T2 ) {
	must( err )
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

func exec( query string, args ...interface{} ) {
	_, err := db.Exec( query, args... )
	if err != nil {
		log.Fatalf( "%+v: %s", err, query )
	}
}

func query( query string, args ...interface{} ) error {
	_, err := db.Exec( query, args... )
	return err
}

func initTables() {
	const application_id = -133015034
	const schema_version = 1

	{
		var id int32
		row := db.QueryRow( "PRAGMA application_id" )
		must( row.Scan( &id ) )

		var version int32
		row = db.QueryRow( "PRAGMA user_version" )
		must( row.Scan( &version ) )

		if id != 0 && version != 0 {
			if id != application_id {
				log.Fatal( "This doesn't look like a ggwiki DB" )
			}

			if version != schema_version {
				log.Fatal( "Guess we gotta migrate lol" )
			}
		}
	}

	exec( fmt.Sprintf( "PRAGMA application_id = %d", application_id ) )
	exec( fmt.Sprintf( "PRAGMA user_version = %d", schema_version ) )

	exec( "PRAGMA foreign_keys = ON" )
	exec( "PRAGMA journal_mode = WAL" )
	exec( "PRAGMA synchronous = NORMAL" )

	{
		// TODO: favicon from first photo
		q := `
		CREATE TABLE IF NOT EXISTS albums (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE CHECK( name <> '' ),
			readonly_url TEXT NOT NULL UNIQUE CHECK( readonly_url <> '' ),
			readwrite_url TEXT NOT NULL UNIQUE CHECK( length( readwrite_url ) >= 16 )
		) STRICT
		`
		exec( q ) // TODO: constrain these to never overlap? or maybe we can pick in the app which one has the tighest constraints and use that.
	}

	{
		q := `
		CREATE TABLE IF NOT EXISTS auto_assign_rules (
			id INTEGER PRIMARY KEY,
			album_id INTEGER NOT NULL,
			start_date INTEGER NOT NULL,
			end_date INTEGER NOT NULL,
			latitude REAL NOT NULL CHECK( latitude >= -90 AND latitude <= 90 ),
			longitude REAL NOT NULL CHECK( longitude >= -180 AND longitude < 180 ),
			radius REAL NOT NULL CHECK( radius >= 0 ),
			CHECK( end_date >= start_date ),
			FOREIGN KEY( album_id ) REFERENCES albums( id )
		) STRICT
		`
		exec( q )
	}

	{
		q := `
		CREATE TABLE IF NOT EXISTS photos (
			id INTEGER PRIMARY KEY,
			album_id INTEGER NULL,
			filename TEXT NOT NULL,
			date INTEGER NULL,
			latitude REAL NULL CHECK( latitude >= -90 AND latitude <= 90 ),
			longitude REAL NULL CHECK( longitude >= -180 AND longitude < 180 ),
			sha256 BLOB NOT NULL CHECK( length( sha256 ) = 32 ),
			thumbhash BLOB NOT NULL,
			thumbnail BLOB NOT NULL,
			image BLOB NOT NULL,
			FOREIGN KEY( album_id ) REFERENCES albums( id )
		) STRICT
		`
		exec( q )
	}

	exec( "INSERT INTO albums ( name, readonly_url, readwrite_url ) VALUES ( 'France 2024', 'france-2024', 'aaaaaaaaaaaaaaaa' ), ( 'Helsinki 2024', 'helsinki-2024', 'bbbbbbbbbbbbbbbb' )" )
	exec( "INSERT INTO auto_assign_rules ( album_id, start_date, end_date, latitude, longitude, radius ) VALUES ( 2, strftime( '%s', '2024-01-01' ), strftime( '%s', '2024-12-31' ), 60.1699, 24.9384, 50 )" )

	{
		f := must1( os.Open( "DSCF2994.jpeg" ) )
		defer f.Close()

		jpg := must1( io.ReadAll( f ) )

		must( addPhotoToAlbum( jpg, 2, "DSCF2994.jpeg" ) )
	}

	{
		f := must1( os.Open( "DSCN0025.jpg" ) )
		defer f.Close()

		jpg := must1( io.ReadAll( f ) )

		must( addPhotoToAlbum( jpg, 2, "DSCN0025.jpg" ) )
	}

	{
		f := must1( os.Open( "776AE6EC-FBF4-4549-BD58-5C442DA2860D.JPG" ) )
		defer f.Close()

		jpg := must1( io.ReadAll( f ) )

		must( addPhoto( jpg, sql.Null[ int64 ] { }, "776AE6EC-FBF4-4549-BD58-5C442DA2860D.JPG" ) )
	}

	// {
	//  tx, err := db.Begin()
	//  if err != nil {
	//      log.Fatal( err )
	//  }
	//
	//  execTx( tx, "INSERT INTO pages ( title, content ) VALUES ( 'Sidebar meta page', 'gg' )" )
	//  TODO: this errors anyway?????? lol??????????????
	//  execTx( tx, "INSERT OR ROLLBACK INTO urls ( url, page_id ) VALUES ( 'Sidebar', last_insert_rowid() )" )
	//
	//  err = tx.Commit()
	//  if err != nil {
	//      log.Fatal( err )
	//  }
	// }
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

func getChecksum( w http.ResponseWriter, r *http.Request, route []string ) {
	io.WriteString( w, checksum )
}

func getImage( w http.ResponseWriter, r *http.Request, route []string ) {
	var filename string
	var image []byte
	{
		raw_sha256 := try1( hex.DecodeString( route[ 0 ] ) )
		row := db.QueryRow( "SELECT filename, image FROM photos WHERE sha256 = $1", raw_sha256 )
		err := row.Scan( &filename, &image )
		if err != nil {
			if errors.Is( err, sql.ErrNoRows ) {
				httpError( w, http.StatusNotFound )
				return
			}
			log.Fatal( err )
		}
	}

	w.Header().Set( "Content-Disposition", fmt.Sprintf( "inline; filename=\"%s\"", filename ) )
	w.Header().Set( "Content-Type", "image/jpeg" )
	w.Write( image )
}

func getThumbnail( w http.ResponseWriter, r *http.Request, route []string ) {
	var thumbnail []byte
	{
		raw_sha256 := try1( hex.DecodeString( route[ 0 ] ) )
		row := db.QueryRow( "SELECT thumbnail FROM photos WHERE sha256 = $1", raw_sha256 )
		err := row.Scan( &thumbnail )
		if err != nil {
			if errors.Is( err, sql.ErrNoRows ) {
				httpError( w, http.StatusNotFound )
				return
			}
			log.Fatal( err )
		}
	}

	w.Header().Set( "Content-Type", "image/jpeg" )
	_ = try1( w.Write( thumbnail ) )
}

func viewAlbum( w http.ResponseWriter, r *http.Request, route []string ) {
	var id int64
	var name string
	var readonly bool
	{
		row := db.QueryRow( "SELECT id, name, readonly_url = $1 FROM albums WHERE readonly_url = $1 OR readwrite_url = $1", route[ 0 ] )
		err := row.Scan( &id, &name, &readonly )
		if err != nil {
			if errors.Is( err, sql.ErrNoRows ) {
				httpError( w, http.StatusNotFound )
				return
			}
			log.Fatal( err )
		}
	}

	type Photo struct {
		SHA256 string
		Thumbhash string
	}

	var photos []Photo
	{
		rows := try1( db.Query( "SELECT filename, sha256, thumbhash FROM photos WHERE album_id = $1 ORDER BY date ASC", id ) )
		defer rows.Close()

		for rows.Next() {
			var filename string
			var sha256 []byte
			var thumbhash []byte
			try( rows.Scan( &filename, &sha256, &thumbhash ) )

			photos = append( photos, Photo {
				SHA256: hex.EncodeToString( sha256 ),
				Thumbhash: base64.StdEncoding.EncodeToString( thumbhash ),
			} )
		}
	}

	context := struct {
		Title string
		AlpineJS template.JS
		ThumbhashJS template.JS
		AlbumURL string
		Checksum string
		Photos []Photo
	}{
		Title: name,
		AlpineJS: template.JS( alpinejs ),
		ThumbhashJS: template.JS( thumbhashjs ),
		AlbumURL: route[ 0 ],
		Checksum: checksum,
		Photos: photos,
	}

	try( templates.ExecuteTemplate( w, "album.html", context ) )
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

func addPhoto( data []byte, album_id sql.Null[ int64 ], filename string ) error {
	decoded, err := StbLoad( data )
	if err != nil {
		return err
	}

	sha256 := sha256.Sum256( data )

	var date sql.Null[ time.Time ]
	var latitude sql.Null[ float64 ]
	var longitude sql.Null[ float64 ]
	orientation := ExifOrientation_Identity

	exif, err := exif.Decode( bytes.NewReader( data ) )
	if err == nil {
		maybe_orientation, err := exifOrientation( exif )
		if err == nil {
			orientation = maybe_orientation
		}

		maybe_date, err := exif.DateTime()
		if err == nil {
			date = sql.Null[ time.Time ] { maybe_date, true }
		}

		maybe_latitude, maybe_longitude, err := exif.LatLong()
		if err == nil {
			latitude = sql.Null[ float64 ] { maybe_latitude, true }
			longitude = sql.Null[ float64 ] { maybe_longitude, true }
		}
	}

	if !album_id.Valid && date.Valid && latitude.Valid {
		rows := try1( db.Query( "SELECT album_id, latitude, longitude, radius FROM auto_assign_rules WHERE start_date <= $1 AND end_date >= $1 ORDER BY end_date - start_date ASC", date.V.Unix() ) )
		defer rows.Close()

		for rows.Next() {
			var id int64
			var rule_latitude float64
			var rule_longitude float64
			var radius float64
			try( rows.Scan( &id, &rule_latitude, &rule_longitude, &radius ) )

			if distance( LatLong { latitude.V, longitude.V }, LatLong { rule_latitude, rule_longitude } ) > radius {
				continue
			}

			album_id = sql.Null[ int64 ] { id, true }
			break
		}
	}

	const thumbnail_size = 512.0

	reoriented := reorient( decoded, orientation )

	scale := thumbnail_size / float32( min( reoriented.Rect.Dx(), reoriented.Rect.Dy() ) )
	thumbnail := StbResize( reoriented, int( float32( reoriented.Rect.Dx() ) * scale ), int( float32( reoriented.Rect.Dy() ) * scale ) )
	thumbnail_jpg := must1( StbToJpg( thumbnail, 85 ) )

	thumbhash := thumbhash.EncodeImage( reoriented )

	q := `
	INSERT OR IGNORE INTO photos ( album_id, filename, date, latitude, longitude, sha256, thumbhash, thumbnail, image )
	VALUES ( $1, $2, $3, $4, $5, $6, $7, $8, $9 )
	`

	err = query( q, album_id, filename, sql.Null[ int64 ] { date.V.Unix(), date.Valid }, latitude, longitude, sha256[ : ], thumbhash, thumbnail_jpg, data )
	if err != nil {
		return err
	}

	return nil
}

func addPhotoToAlbum( data []byte, album_id int64, filename string ) error {
	return addPhoto( data, sql.Null[ int64 ] { album_id, true }, filename )
}

func uploadPhotos( w http.ResponseWriter, r *http.Request, route []string ) {
	const megabyte = 1000 * 1000
	try( r.ParseMultipartForm( 10 * megabyte ) )

	fmt.Printf( "%d\n", len( r.MultipartForm.File[ "photos" ] ) )
	for _, header := range r.MultipartForm.File[ "photos" ] {
		f := try1( header.Open() )
		defer f.Close()

		contents := try1( io.ReadAll( f ) )

		fmt.Printf( "%s -> %d\n", header.Filename, len( contents ) )
	}

	http.Redirect( w, r, r.URL.Path, http.StatusSeeOther )
}

func admin( w http.ResponseWriter, r *http.Request, route []string ) {
}

type Route struct {
	Url *regexp.Regexp
	Method string
	Handler func( http.ResponseWriter, *http.Request, []string )
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
			if matches := route.Url.FindStringSubmatch( r.URL.Path ); len( matches ) > 0 {
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
		Addr:    addr,
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

func templateAdd( a int, b int ) int {
	return a + b
}

func main() {
	checksum = exeChecksum()

	template_funcs := template.FuncMap{
		"add": templateAdd,
	}
	templates = must1( template.New( "dummy" ).Funcs( template_funcs ).ParseFS( template_sources, "*" ) )

	{
		path := "file::memory:?cache=shared"
		if len( os.Args ) == 2 {
			path = os.Args[ 1 ]
		} else {
			fmt.Println( "Using in memory database. Nothing will be saved when you quit the server!" )
		}

		var err error
		db, err = sql.Open( "sqlite3", path )
		if err != nil {
			log.Fatal( err )
		}
	}
	defer db.Close()

	initTables()

	{
		m := minify.New()
		// m.AddFunc( "css", minify_css.Minify )
		m.AddFunc( "js", minify_js.Minify )

		thumbhashjs = must1( m.String( "js", strings.ReplaceAll( thumbhashjs, "export function", "function" ) ) )
	}

	public_http_server := startHttpServer( "127.0.0.1:5678", []Route {
		{ regexp.MustCompile( "^/Special:checksum$" ), "GET", getChecksum },
		{ regexp.MustCompile( "^/Special:image/(.+)$" ), "GET", getImage },
		{ regexp.MustCompile( "^/Special:thumbnail/(.+)$" ), "GET", getThumbnail },
		// { regexp.MustCompile( "^/Special:geocode$" ), "GET", geocode },
		{ regexp.MustCompile( "^/([^:]+)$" ), "GET", viewAlbum },
		{ regexp.MustCompile( "^/([^:]+)$" ), "POST", uploadPhotos },
	} )

	private_http_server := startHttpServer( "127.0.0.1:12345", []Route {
		{ regexp.MustCompile( "^/$" ), "GET", admin },
	} )

	done := make( chan os.Signal, 1 )
	signal.Notify( done, syscall.SIGINT, syscall.SIGTERM )
	<-done

	ctx, cancel := context.WithTimeout( context.Background(), 5 * time.Second )
	defer func() {
		db.Close()
		cancel()
	}()

	must( public_http_server.Shutdown( ctx ) )
	must( private_http_server.Shutdown( ctx ) )
}
