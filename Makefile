GOFLAGS=
TAGS=fts5

ifeq ($(shell uname -s),Linux)
	TAGS += sqlite_omit_load_extension osusergo netgo
	GOFLAGS += -ldflags="-extldflags=-static" -tags "$(TAGS)" -gcflags "-N -l"
endif

all:
	sqlc generate
	templ generate -path src
	go build -C src -o ../mikegram $(GOFLAGS)

release: TAGS += release
release: all

clean:
	rm mikegram
