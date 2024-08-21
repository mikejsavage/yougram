package main

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/adler32"
	"hash/crc32"
	"hash/fnv"
	"html/template"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/galdor/go-thumbhash"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/tdewolff/minify/v2"
	minify_css "github.com/tdewolff/minify/v2/css"
	minify_js "github.com/tdewolff/minify/v2/js"
)

var db *sql.DB

// //go:embed style.css
// var stylesheet string
//
// //go:embed main.js
// var js string
//
// //go:embed favicon16.png
// var favicon16 []byte
// var favicon16_base64 string
//
// //go:embed logo_minified.svg
// var logo string

//go:embed *.html
var template_sources embed.FS
var templates *template.Template

//go:embed *.js
var js embed.FS
var alpinejs string
var thumbhashjs string

var checksum string

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

func initTables() {
	const application_id = -1524217918
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
			date INTEGER NOT NULL,
			latitude REAL NULL CHECK( latitude >= -90 AND latitude <= 90 ),
			longitude REAL NULL CHECK( longitude >= -180 AND longitude < 180 ),
			sha256 BLOB NOT NULL CHECK( length( sha256 ) = 32 ),
			thumbhash BLOB NOT NULL,
			thumbnail BLOB NOT NULL,
			FOREIGN KEY( album_id ) REFERENCES albums( id )
		) STRICT
		`
		exec( q )
	}

	exec( "INSERT INTO albums ( name, readonly_url, readwrite_url ) VALUES ( 'France 2024', 'france-2024', 'aaaaaaaaaaaaaaaa' ), ( 'Helsinki 2024', 'helsinki-2024', 'bbbbbbbbbbbbbbbb' )" )
	exec( "INSERT INTO auto_assign_rules ( album_id, start_date, end_date, latitude, longitude, radius ) VALUES ( 2, strftime( '%s', '2024-01-01' ), strftime( '%s', '2024-12-31' ), 60, 24, 50 )" )

	{
		f := must1( os.Open( "DSCF2994.jpeg" ) )
		defer f.Close()

		decoded := must1( StbLoad( f ) )
		scale := 512.0 / float32( min( decoded.Rect.Dx(), decoded.Rect.Dy() ) )
		thumbnail := StbResize( decoded, int( float32( decoded.Rect.Dx() ) * scale ), int( float32( decoded.Rect.Dy() ) * scale ) )
		thumbnail_jpg := must1( StbToJpg( thumbnail, 80 ) )

		thumbnail2 := thumbhash.EncodeImage( decoded )

		exec( "INSERT INTO photos ( album_id, filename, date, latitude, longitude, sha256, thumbhash, thumbnail ) VALUES ( 1, 'DSCF2994.jpeg', strftime( '%s', '2024-12-31' ), 0, 0, x'cc85f99cd694c63840ff359e13610390f85c4ea0b315fc2b033e5839e7591949', $1, $2 )", thumbnail2, thumbnail_jpg )
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

func http404( w http.ResponseWriter ) {
	http.Error( w, "Not Found", http.StatusNotFound )
}

func getChecksum( w http.ResponseWriter, r *http.Request, route []string ) {
	io.WriteString( w, checksum )
}

func getImage( w http.ResponseWriter, r *http.Request, route []string ) {
	var album_name string
	var image_filename string
	{
		raw_sha256 := try1( hex.DecodeString( route[ 0 ] ) )
		row := db.QueryRow( "SELECT albums.name, photos.filename FROM photos, albums WHERE photos.sha256 = $1 AND albums.id = photos.album_id", raw_sha256 )
		err := row.Scan( &album_name, &image_filename )
		if err != nil {
			if errors.Is( err, sql.ErrNoRows ) {
				http404( w )
				return
			}
			log.Fatal( err )
		}
	}

	w.Header().Set( "Content-Disposition", fmt.Sprintf( "inline; filename=\"%s\"", image_filename ) )
	w.Header().Set( "Content-Type", "image/jpeg" )
	http.ServeFile( w, r, filepath.Join( album_name, image_filename ) )
}

func getThumbnail( w http.ResponseWriter, r *http.Request, route []string ) {
	var thumbnail []byte
	{
		raw_sha256 := try1( hex.DecodeString( route[ 0 ] ) )
		row := db.QueryRow( "SELECT thumbnail FROM photos WHERE sha256 = $1", raw_sha256 )
		err := row.Scan( &thumbnail )
		if err != nil {
			if errors.Is( err, sql.ErrNoRows ) {
				http404( w )
				return
			}
			log.Fatal( err )
		}
	}

	w.Header().Set( "Content-Type", "image/jpeg" )
	_ = try1( w.Write( thumbnail ) )
}

func pngCrc( data []byte ) []byte {
	hasher := crc32.NewIEEE()
	hasher.Write( data )
	return hasher.Sum( nil )
}

func thumbhashToBase64Png( thumb []byte ) string {
	decoded := try1( thumbhash.DecodeImage( thumb ) ).( *image.RGBA )

	// https://github.com/evanw/thumbhash/blob/a652ce6ed691242f459f468f0a8756cda3b90a82/js/thumbhash.js#L234
	// https://stackoverflow.com/a/69353504 - lots of bad stuff, mixed dec/hex, "IDHR", zlib header is "120 1" not "0 0"
	png := new( strings.Builder )
	png.Write( []byte{ 137, 80, 78, 71, 13, 10, 26, 10 } )

	{
		ihdr := new( bytes.Buffer )
		ihdr.Write( []byte {
			'I', 'H', 'D', 'R',
		} )
		binary.Write( ihdr, binary.BigEndian, uint32( decoded.Rect.Dx() ) )
		binary.Write( ihdr, binary.BigEndian, uint32( decoded.Rect.Dy() ) )
		ihdr.Write( []byte {
			8,
			6,
			0,
			0,
			0,
		} )

		binary.Write( png, binary.BigEndian, uint32( ihdr.Len() - 4 ) )
		png.Write( ihdr.Bytes() )
		png.Write( pngCrc( ihdr.Bytes() ) )
	}

	{
		idat := new( bytes.Buffer )
		idat.Write( []byte {
			'I', 'D', 'A', 'T',
			120, 1,
			1,
		} )

		data_length16 := uint16( decoded.Rect.Dx() * decoded.Rect.Dy() * 4 )
		binary.Write( idat, binary.BigEndian, data_length16 )
		binary.Write( idat, binary.BigEndian, ^data_length16 )

		for y := 0; y < decoded.Rect.Dy(); y++ {
			idat.Write( []byte { 0 } )
			idat.Write( decoded.Pix[ y * decoded.Stride : ( y + 1 ) * decoded.Stride ] )
		}

		hasher := adler32.New()
		hasher.Write( []byte( decoded.Pix ) )
		idat.Write( hasher.Sum( nil ) )

		binary.Write( png, binary.BigEndian, uint32( idat.Len() - 4 ) )
		png.Write( idat.Bytes() )
		png.Write( pngCrc( idat.Bytes() ) )
	}

	png.Write( []byte {
		00, 00, 00, 00,
		'I', 'E', 'N', 'D',
		174, 66, 96, 130,
	} )

	try( os.WriteFile( "gg.png", []byte( png.String() ), 0644 ) )

	return base64.StdEncoding.EncodeToString( []byte( png.String() ) )
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
				http404( w )
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
	Url     *regexp.Regexp
	Method  string
	Handler func( http.ResponseWriter, *http.Request, []string )
}

func startHttpServer( addr string, routes []Route ) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc( "/", func( w http.ResponseWriter, r *http.Request ) {
		defer func() {
			if r := recover(); r != nil {
				log.Print( r )
				http.Error( w, "Internal Server Error", http.StatusInternalServerError )
				return
			}
		}()

		is405 := false

		for _, route := range routes {
			if matches := route.Url.FindStringSubmatch( r.URL.Path ); len( matches ) > 0 {
				if r.Method == route.Method {
					route.Handler( w, r, matches[1:] )
					return
				}

				is405 = true
			}
		}

		if is405 {
			http.Error( w, "Method Not Allowed", http.StatusMethodNotAllowed )
		} else {
			http.Error( w, "Not Found", http.StatusNotFound )
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

	{
		f := must1( os.Open( "DSCN0025.jpg" ) )
		defer f.Close()

		exif := must1( exif.Decode( f ) )
		lat, long := must2( exif.LatLong() )
		fmt.Printf( "%f %f\n", lat, long )

		_ = must1( f.Seek( 0, io.SeekStart ) )

		decoded := must1( StbLoad( f ) )
		thumbnail := StbResize( decoded, 456, 123 )

		must( StbWriteJpg( "thumbnail.jpg", thumbnail, 80 ) )
	}

	initTables()

	{
		m := minify.New()
		m.AddFunc( "css", minify_css.Minify )
		m.AddFunc( "js", minify_js.Minify )

		alpinejs = must1( m.String( "js", string( must1( js.ReadFile( "alpine-3.14.1.js" ) ) ) ) )
		thumbhashjs = must1( m.String( "js", strings.ReplaceAll( string( must1( js.ReadFile( "thumbhash.js" ) ) ), "export function", "function" ) ) )
	}

	public_http_server := startHttpServer( "127.0.0.1:5678", []Route {
		{ regexp.MustCompile( "^/Special:checksum$" ), "GET", getChecksum },
		{ regexp.MustCompile( "^/Special:image/( .+ )$" ), "GET", getImage },
		{ regexp.MustCompile( "^/Special:thumbnail/( .+ )$" ), "GET", getThumbnail },
		// { regexp.MustCompile( "^/Special:geocode$" ), "GET", geocode },
		{ regexp.MustCompile( "^/( [^:]+ )$" ), "GET", viewAlbum },
		{ regexp.MustCompile( "^/( [^:]+ )$" ), "POST", uploadPhotos },
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
