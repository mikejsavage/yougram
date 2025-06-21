GOFLAGS=

ifeq ($(shell uname -s),Linux)
	GOFLAGS += -ldflags="-extldflags=-static" -tags sqlite_omit_load_extension,osusergo,netgo -gcflags "-N -l"
endif

all:
	sqlc generate
	templ generate -path src
	go build -C src -o ../mikegram $(GOFLAGS) -tags fts5

release:
	sqlc generate
	templ generate -path src
	go build -C src -o ../mikegram $(GOFLAGS) -tags "fts5 release"

mikegram: src/* src/stb/* src/sqlc/*
	templ generate -path src
	go build -C src -o ../mikegram $(GOFLAGS)

src/sqlc/query.sql.go: src/schema.sql src/query.sql
	sqlc generate

clean:
	rm mikegram

get:
	go get mikegram

update:
	go get -u

tidy:
	go mod tidy
