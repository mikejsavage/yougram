This folder contains precompiled ffmpeg bins. You can build them yourself by cloning the following:

- https://github.com/allyourcodebase/ffmpeg
- https://github.com/allyourcodebase/libmp3lame
- https://github.com/allyourcodebase/libogg
- https://github.com/allyourcodebase/libvorbis
- https://github.com/allyourcodebase/zlib

and build them like so (assumes you clone all the dependencies into the ffmpeg dir):

```sh
#! /bin/sh

f() {
    zig build -Doptimize=ReleaseSmall -Dtarget=$1

    cd libmp3lame
    zig build -Doptimize=ReleaseFast -Dtarget=$1

    cd ../libogg
    zig build -Doptimize=ReleaseFast -Dtarget=$1

    cd ../libvorbis
    zig build -Doptimize=ReleaseFast -Dtarget=$1

    cd ../zlib
    zig build -Doptimize=ReleaseFast -Dtarget=$1

    cd ..
    cp zig-out/lib/libffmpeg.a blah/libffmpeg_$2.a
    cp libmp3lame/zig-out/lib/libmp3lame.a blah/libmp3lame_$2.a
    cp libogg/zig-out/lib/libogg.a blah/libogg_$2.a
    cp libvorbis/zig-out/lib/libvorbis.a blah/libvorbis_$2.a
    cp zlib/zig-out/lib/libz.a blah/libz_$2.a
}

mkdir -p blah
f x86_64-linux-musl linux_amd64
f aarch64-linux-musl linux_arm64
f aarch64-macos darwin_arm64
```

ffmpeg `ReleaseSmall` enables specific size optimisations in ffmpeg that make it much smaller,
`ReleaseFast` builds are too big for GitHub...
