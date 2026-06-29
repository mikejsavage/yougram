linux_tags := "osusergo netgo"
linux_ldflags := "-extldflags=-static"

os_tags := if os() == "linux" { linux_tags } else { "" }
os_ldflags := if os() == "linux" { linux_ldflags } else { "" }
os_debug_goflags := if os() == "linux" { "-gcflags \"-N -l\"" } else { "" }

dev: (_yougram "-dev" os_debug_goflags "" "")
release: (_yougram "" "" "-s -w" "release")

_yougram bin_suffix config_goflags config_ldflags config_tags:
	@# 20260629: these don't work on NixOS
	@# go tool -C src sqlc generate
	@# go tool -C src templ generate
	cd src && sqlc generate
	templ generate -path src
	env GO_CFLAGS=-O2 go build -C src -o ../yougram{{bin_suffix}} \
		{{config_goflags}} \
		-ldflags="{{os_ldflags}} {{config_ldflags}}" \
		-tags "fts5 nodynamic sqlite_omit_load_extension {{os_tags}} {{config_tags}}"

[macos]
package:
	@# TODO 20260629: go strip ldflags don't actually strip when cross compiling. zig objcopy removed
	@# stripping for some reason but we can use that when they readd it
	@just _yougram "_macos_arm64" "" "-s -w" "release"
	@env GOOS=linux GOARCH=amd64 CC="zig cc -target x86_64-linux" CXX="zig c++ -target x86_64-linux" just _yougram "_linux_amd64" "" "-s -w {{linux_ldflags}}" "release {{linux_tags}}"
	@env GOOS=linux GOARCH=arm64 CC="zig cc -target aarch64-linux" CXX="zig c++ -target aarch64-linux" just _yougram "_linux_arm64" "" "-s -w {{linux_ldflags}}" "release {{linux_tags}}"

clean:
	rm -f yougram yougram-dev
