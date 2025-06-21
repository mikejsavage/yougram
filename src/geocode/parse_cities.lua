#! /usr/bin/env lua

local function split( str, delimeter )
	local res = { }
	for segment in str:gmatch( "[^" .. delimeter .. "]*" ) do
		table.insert( res, segment )
	end
	return table.unpack( res )
end

local function printf( form, ... )
	print( form:format( ... ) )
end

print( [[
CREATE TABLE city (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	alternative_names TEXT NOT NULL,
	country TEXT NOT NULL,
	latitude REAL NOT NULL,
	longitude REAL NOT NULL,
	population INTEGER NOT NULL
) STRICT;

CREATE VIRTUAL TABLE geocode USING fts5( name, alternative_names, content=city, content_rowid=id );
]] )

-- https://download.geonames.org/export/dump/
for line in io.lines( "cities5000.txt" ) do
	local id, name, ascii_name, alt_names,
		latitude, longitude, idk, idk2,
		country, cc2, admin1, admin2, admin3, admin4,
		population, elevation, dem, timezone, modtime = split( line, "\t" )
	local all_alt_names = ascii_name .. "," .. alt_names
	printf( [[
		INSERT INTO city ( name, alternative_names, country, latitude, longitude, population )
		VALUES ( "%s", "%s", "%s", %s, %s, %s );
	]], name, all_alt_names, country, latitude, longitude, population )
	printf( [[
		INSERT INTO geocode ( rowid, name, alternative_names )
		VALUES( last_insert_rowid(), "%s", "%s" );
	]], name, all_alt_names )
end
