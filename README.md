yougram
-------

yougram is a self-hosted image sharing app.

Please note that yougram is very early in development and basically unusable. Parts of this readme
have been written for an idealised future yougram that doesn't exist yet, so expect contradictions
and falsehoods.


## Why yougram is good for you, a user

- Trivial to deploy: copy a single file to your server with zero dependencies
- Set and forget: yougram will never make breaking changes and doesn't depend on external services
  that may disappear in the future. If you like it today you will like it a decade from now too
  (not true yet)
- Multi-user: your family can use it too
- (optionally) AI powered: AI photo tagging so you can search for "cat", and facial recognition so
  you can search for "Mike" (not true yet)
- Guest access: share secret album links with your friends, both writable (for group vacations) and
  read-only (for everyone else)
- Backup friendly: yougram stores your data unmodified as files on disk, so it works well with
  standard backup solutions (restic/borg/etc)
- No lock-in: getting your data out of yougram is an explicitly supported and documented workflow,
  feel free to take your photos elsewhere (not really true yet)
- Private: nothing leaves your computer
- Compatible with the Immich app: automatically upload your phone library to yougram (not true yet)
- Snappy: I'm not a web developer so everything happens instantly
- Scalable: yougram does not currently scale to millions of photos, but tens of thousands is ok
- RAW support: you can upload and download RAWs and they stack with your JPEGs but that's about it
  (also not true yet)
- Video support: not yet


## Installation instructions

[funnel]: https://tailscale.com/kb/1223/funnel
[caddy]: https://caddyserver.com
[haproxy]: https://www.haproxy.org

1. Download a binary from GitHub releases and copy it to your server
2. Make a directory somewhere for yougram to store its data and `cd` to it
3. Create a user by running `yougram create-user` and following the prompts
4. Figure out how you want to expose yougram to the internet. yougram is split into a private
   interface and a guest interface. The easiest way to securely expose both is through Tailscale,
   you can share the private interface by making a tailnet for your family, and expose the guest
   interface with [Tailscale Funnel][funnel]. More generally, it is not a good idea to directly
   expose the private interface to the internet, but running it behind a TLS terminating proxy (e.g.
   [Caddy][caddy] or [HAProxy][haproxy]) is ok. Exposing the guest interface directly to the
   internet should be fine.
5. Run it with `yougram --private-interface 0.0.0.0:12345 --guest-interface 0.0.0.0:12346
   --guest-url https://guestgram.example.com`. Remember, yougram stores everything in the current
   working directory, so make sure you're in the right place first!
6. Optionally, if you want AI image classification, download the Moondream model and put it in the
   `moondream` directory.

For a concrete example, my HAProxy config looks like this:

```
frontend haproxy
        bind *:80 v4v6
        bind *:443 v4v6 ssl crt /etc/ssl/private

        acl guestgram hdr(host) -i -m beg guestgram.mikejsavage.co.uk
        acl letsencrypt path_beg /.well-known/acme-challenge/

        use_backend httpd_letsencrypt if letsencrypt
        use_backend guestgram if guestgram
        use_backend httpd

backend httpd
        server httpd 127.0.0.1:8080

backend httpd_letsencrypt
        server httpd 127.0.0.1:8081

backend guestgram
        server guestgram 100.64.0.4:10006
```

and my NAS's HAProxy config contains:

```
frontend yougram
    bind :10004 interface tailscale0 ssl crt /var/lib/acme/vpn.mikejsavage.co.uk/full.pem
    use_backend yougram

backend yougram
    server yougram 127.0.0.1:10005
```

and I run yougram like:

```
./mikegram serve
    --private-listen-addr localhost:10005
    --guest-listen-addr 100.64.0.4:10006
    --guest-url https://guestgram.mikejsavage.co.uk
```

Eventually your yougram directory will look like this:

```
yougram/
    db.sq3 <- contains all the album metadata etc, backing up SQLite databases requires special care so...
    assets/ <- contains all your photos, only back up this folder!
        metadata_backup.json: ...yougram also saves all its metadata here, in a backup friendly format
    generated/ <- contains thumbnails and HEIC -> JPG conversions and the like, this can be entirely regenerated from your assets so you don't need to keep it safe
    moondream/ <- contains AI models for image classification, you downloaded these from the internet so again feel free to kill it
```


## Updating

Download a new binary.


## System requirements

I develop on macOS and Linux. Yougram should run on Windows and other Unixes but I haven't and won't
test them.

Yougram should run on arbitrarily bad hardware, but probably don't enable the AI features on a
Raspberry Pi.

The yougram binary is around 25MB. The optional Moondream AI model is 2GB.


## Security

The private/guest split makes it easy to hide most of yougram behind a VPN, which makes certain
classes of attacks impossible, for example frontend vulnerabilities in the guest interface cannot be
leveraged into stealing accounts. Beyond that I make no claims regarding the security of the app
beyond that I have thought about it a bit.

Probably don't make accounts for untrusted users on your instance, but the guest interface is
legitimately trivial and can be shared freely.


## Why yougram is good for you, a developer

yougram is easy to hack on. I'm not a web developer so the codebase is tiny and dirt simple. If I
can make it work then so can you.

It doesn't have zero dependencies, but it also doesn't have very many and it really is quite easy to
set up a dev environment.


## Techie stuff

The backend stack is Go/SQLC/templ and SQLite. The frontend stack is raw CSS, HTMX, and Alpine.js

If you want to compile yougram yourself, first you need to install a not particularly recent version
(probably anything 2024 or later) of Go. You can do it through your package manager or just
[download it from the website](https://go.dev/doc/install).

You also need a C compiler because yougram uses Cgo. I think this makes it very hard to compile on
Windows, so sorry about that, but macOS and other Unixy systems are fine.

Next, install `sqlc` and `templ`. If you have a recent version of Go, you can install them through
go, like so:

```
go get -tool github.com/a-h/templ/cmd/templ@latest
go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

otherwise your package manager should have them, otherwise see
https://docs.sqlc.dev/en/stable/overview/install.html and
https://templ.guide/quick-start/installation/.

If you're using Nix/Devbox/etc, you can make a dev shell with go/gcc/glibc/sqlc/templ.

Then you should be able to use the Makefile to compile yougram, or by hand:

```
sqlc generate
templ generate -path src
go build -C src -o ../mikegram
```
