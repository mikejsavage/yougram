package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	_ "embed"
	"io"
	"os"
)

//go:embed geocode/geocode.sq3.gz
var geocode_sq3_gz []byte

var geocode_db *sql.DB
var temp_path string

func initGeocoder() {
	gz := must1( gzip.NewReader( bytes.NewReader( geocode_sq3_gz ) ) )

	f := must1( os.CreateTemp( "", "yougram-geocode-*.sq3" ) )
	temp_path = f.Name()

	_ = must1( io.Copy( f, gz ) )
	must( f.Close() )

	geocode_db = must1( sql.Open( "sqlite3", f.Name() + "?mode=ro" ) )
}

func shutdownGeocoder() {
	geocode_db.Close()
	must( os.Remove( temp_path ) )
}

type GeocodeResult struct {
	City string `json:"city"`
	Country string `json:"country"`
	Latitude float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func geocode( search string ) []GeocodeResult {
	const query = `
		SELECT geocode.name, country, Latitude, Longitude
		FROM geocode( ? )
		JOIN city ON geocode.rowid = city.id
		ORDER BY city.population DESC
	`

	rows := must1( geocode_db.Query( query, search ) )
    defer rows.Close()

	var results []GeocodeResult
    for rows.Next() {
        var result GeocodeResult
		must( rows.Scan( &result.City, &result.Country, &result.Latitude, &result.Longitude ) )
        results = append( results, result )
    }

	return results
}
