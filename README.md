yougram
-------

yougram is a self-hosted image app.


## Why yougram is good for you, a user

- Trivial to deploy: copy a single file to your server with zero dependencies
- Multi-user capable: your family can use it too
- AI powered: you can search for "cat" to find all your cat photos
- Guest access: share secret album links with your friends, both writeable (for
  group vacations) and read-only (for everyone else)
- Backup friendly: yougram stores your data unmodified as a bunch of files on
  disk, so it works well with standard backup solutions (restic/borg/etc)
- No lock-in: getting your data out of yougram is an explicitly supported and
  documented workflow, feel free to take your photos elsewhere
- Compatible with the Immich app: automatically upload your phone library to
  yougram
- Snappy: I have realistic expectations of how a photo management app should
  perform, i.e. everything should run instantly, and I know nothing about web
  development so everything actually does run instantly
- Scalable: yougram does not currently scale well to millions of photos, but
  anything less than that is ok


## Installation instructions

1. Download a binary from GitHub releases and copy it to your server
2. Make a directory somewhere for yougram to store its data and `cd` to it
3. Create a user by running `yougram create-user` and following the prompts
4. Figure out how you want to expose yougram to the internet. yougram is split
   into a private interface and a guest interface. The easiest way to securely
   expose both is through Tailscale, you can share the private interface by
   making a tailnet for your family, and expose the guest interface with
   Tailscale Funnel. More generally, it is not a good idea to directly expose
   the private interface to the internet, but running it behind a TLS
   terminating proxy (e.g. HAProxy) is ok. Exposing the guest interface directly
   to the internet should be fine.
5. Run it with `yougram --private-interface 0.0.0.0:12345 --guest-interface
   0.0.0.0:12346 --guest-url https://guestgram.example.com`. Remember, yougram
   stores everything in the current working directory, so make sure you're in
   the right place first!
6. Optionally, if you want AI image classification, download the moondream model
   and put it in the `moondream` directory.

For a concrete example, my HAProxy config looks like this:

```
TODO
```

and I run yougram like `yougram ...`.

Eventually your yougram directory will look like this:

```
yougram/
    db.sq3 <- contains all the album metadata etc, backing up SQLite databases requires special care so...
    assets/ <- contains all your photos, only back up this folder!
        metadata_backup.json: ...yougram also saves all its metadata here, in a backup friendly format
    generated/ <- contains thumbnails and the like, this can be entirely regenerated from your assets so you don't need to keep it safe
    moondream/ <- contains AI models for image classification, you downloaded these from the internet so again feel free to kill it


## Security

The private/guest split makes it easy to hide most of yougram behind a VPN,
which makes certain classes of attacks impossible, for example an XSS
vulnerability in the guest interface cannot be leveraged into stealing accounts.
Beyond that I make no claims regarding the security of the app, other than that

Probably don't invite untrusted users to make an account on your instance, but
the guest interface is legitimately trivial and can be shared freely.


## Why yougram is good for you, a developer

yougram is easy to hack on. I know nothing about web development so the codebase
is tiny and dirt simple. If I can make it work then so can you.

It doesn't quite have zero dependencies, but it also doesn't have very many and
it really is quite easy to set up a dev environment.

## Techie stuff

The backend "stack" is Go/SQLC/templ and SQLite. The frontend "stack" is raw
CSS, HTMX, and Alpine.js

If you want to compile yougram yourself, first you need to install a not
particularly recent version (probably anything 2024 or later) of Go. You can do
it through your package manager or just [download it from the
website](https://go.dev/doc/install).

You also need a C compiler because yougram uses Cgo. I think this makes it very
hard to compile on Windows, so sorry about that, but macOS and other Unixy
systems are fine.

Next, install `sqlc` and `templ`. If you have a recent version of Go, you can
install them through go, like so:

```
go get -tool github.com/a-h/templ/cmd/templ@latest
go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

otherwise your package manager should have them, otherwise see
https://docs.sqlc.dev/en/stable/overview/install.html and
https://templ.guide/quick-start/installation/.

If you're using Nix/Devbox/etc, you can make a dev shell with
go/gcc/glibc/sqlc/templ.

Then you should be able to use the Makefile to compile yougram, or by hand:

```
sqlc generate
templ generate -path src
go build -C src -o ../mikegram
```
