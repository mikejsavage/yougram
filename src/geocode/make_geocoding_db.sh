#! /bin/sh

wget https://download.geonames.org/export/dump/cities5000.zip
7z x cities5000.zip
rm cities5000.zip

lua parse_cities.lua > cities.sql
sqlite3 -init cities.sql geocode.sq3 ".quit"
gzip -9 -k geocode.sq3
