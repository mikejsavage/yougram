BIN_SUFFIX=-dev
TAGS=fts5 nodynamic
GOFLAGS=
LDFLAGS=

ifeq ($(shell uname -s),Linux)
	TAGS += sqlite_omit_load_extension osusergo netgo
	GOFLAGS += -gcflags "-N -l"
	LDFLAGS += -extldflags=-static
endif

all:
	cd src && sqlc generate
	templ generate -path src
	env GO_CFLAGS=-O2 go build -C src -o ../yougram$(BIN_SUFFIX) $(GOFLAGS) -ldflags="$(LDFLAGS)" -tags "$(TAGS)"

release: BIN_SUFFIX =
release: TAGS += release
release: LDFLAGS += -s -w
release: all

clean:
	rm -f yougram yougram-dev
