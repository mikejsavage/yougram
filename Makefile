GOFLAGS=

ifeq ($(shell uname -s),Linux)
	GOFLAGS += -ldflags="-extldflags=-static" -tags sqlite_omit_load_extension,osusergo,netgo -gcflags "-N -l"
endif

all: src/*
	go build -C src -o ../mikegram $(GOFLAGS)

clean:
	rm mikegram

get:
	go get mikegram

update:
	go get -u

tidy:
	go mod tidy
