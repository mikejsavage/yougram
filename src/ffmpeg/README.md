This folder contains precompiled ffmpeg bins. You can build them yourself by cloning the following:

- https://github.com/allyourcodebase/ffmpeg
- https://github.com/allyourcodebase/libmp3lame
- https://github.com/allyourcodebase/libogg
- https://github.com/allyourcodebase/libvorbis
- https://github.com/allyourcodebase/zlib

then `git apply` this patch to ffmpeg:

```diff
diff --git a/build.zig b/build.zig
index 10a0312714..7f8a261230 100644
--- a/build.zig
+++ b/build.zig
@@ -718,7 +718,7 @@ pub fn build(b: *std.Build) void {
         .CONFIG_FAST_UNALIGNED = fastUnalignedLoads(t),
         .CONFIG_LSP = true,
         .CONFIG_PIXELUTILS = true,
-        .CONFIG_NETWORK = true,
+        .CONFIG_NETWORK = false,
         .CONFIG_AUTODETECT = false,
         .CONFIG_FONTCONFIG = false,
         .CONFIG_LARGE_TESTS = true,
@@ -2742,11 +2742,11 @@ pub fn build(b: *std.Build) void {
         .CONFIG_RPL_DEMUXER = true,
         .CONFIG_RSD_DEMUXER = true,
         .CONFIG_RSO_DEMUXER = true,
-        .CONFIG_RTP_DEMUXER = true,
-        .CONFIG_RTSP_DEMUXER = true,
+        .CONFIG_RTP_DEMUXER = false,
+        .CONFIG_RTSP_DEMUXER = false,
         .CONFIG_S337M_DEMUXER = true,
         .CONFIG_SAMI_DEMUXER = true,
-        .CONFIG_SAP_DEMUXER = true,
+        .CONFIG_SAP_DEMUXER = false,
         .CONFIG_SBC_DEMUXER = true,
         .CONFIG_SBG_DEMUXER = true,
         .CONFIG_SCC_DEMUXER = true,
@@ -3003,10 +3003,10 @@ pub fn build(b: *std.Build) void {
         .CONFIG_RM_MUXER = true,
         .CONFIG_ROQ_MUXER = true,
         .CONFIG_RSO_MUXER = true,
-        .CONFIG_RTP_MUXER = true,
+        .CONFIG_RTP_MUXER = false,
         .CONFIG_RTP_MPEGTS_MUXER = true,
-        .CONFIG_RTSP_MUXER = true,
-        .CONFIG_SAP_MUXER = true,
+        .CONFIG_RTSP_MUXER = false,
+        .CONFIG_SAP_MUXER = false,
         .CONFIG_SBC_MUXER = true,
         .CONFIG_SCC_MUXER = true,
         .CONFIG_SEGAFILM_MUXER = true,
@@ -3055,14 +3055,14 @@ pub fn build(b: *std.Build) void {
         .CONFIG_DATA_PROTOCOL = true,
         .CONFIG_FD_PROTOCOL = true,
         .CONFIG_FFRTMPCRYPT_PROTOCOL = false,
-        .CONFIG_FFRTMPHTTP_PROTOCOL = true,
+        .CONFIG_FFRTMPHTTP_PROTOCOL = false,
         .CONFIG_FILE_PROTOCOL = true,
-        .CONFIG_FTP_PROTOCOL = true,
-        .CONFIG_GOPHER_PROTOCOL = true,
+        .CONFIG_FTP_PROTOCOL = false,
+        .CONFIG_GOPHER_PROTOCOL = false,
         .CONFIG_GOPHERS_PROTOCOL = false,
         .CONFIG_HLS_PROTOCOL = true,
-        .CONFIG_HTTP_PROTOCOL = true,
-        .CONFIG_HTTPPROXY_PROTOCOL = true,
+        .CONFIG_HTTP_PROTOCOL = false,
+        .CONFIG_HTTPPROXY_PROTOCOL = false,
         .CONFIG_HTTPS_PROTOCOL = tls != .disabled,
         .CONFIG_ICECAST_PROTOCOL = true,
         .CONFIG_MMSH_PROTOCOL = true,
@@ -3076,16 +3076,16 @@ pub fn build(b: *std.Build) void {
         .CONFIG_RTMPT_PROTOCOL = true,
         .CONFIG_RTMPTE_PROTOCOL = false,
         .CONFIG_RTMPTS_PROTOCOL = false,
-        .CONFIG_RTP_PROTOCOL = true,
+        .CONFIG_RTP_PROTOCOL = false,
         .CONFIG_SCTP_PROTOCOL = false,
-        .CONFIG_SRTP_PROTOCOL = true,
+        .CONFIG_SRTP_PROTOCOL = false,
         .CONFIG_SUBFILE_PROTOCOL = true,
         .CONFIG_TEE_PROTOCOL = true,
-        .CONFIG_TCP_PROTOCOL = true,
+        .CONFIG_TCP_PROTOCOL = false,
         .CONFIG_TLS_PROTOCOL = tls != .disabled,
-        .CONFIG_UDP_PROTOCOL = true,
-        .CONFIG_UDPLITE_PROTOCOL = true,
-        .CONFIG_UNIX_PROTOCOL = true,
+        .CONFIG_UDP_PROTOCOL = false,
+        .CONFIG_UDPLITE_PROTOCOL = false,
+        .CONFIG_UNIX_PROTOCOL = false,
         .CONFIG_LIBAMQP_PROTOCOL = false,
         .CONFIG_LIBRIST_PROTOCOL = false,
         .CONFIG_LIBRTMP_PROTOCOL = false,
@@ -6107,7 +6107,7 @@ const all_sources = [_][]const u8{
     "libavformat/hlsplaylist.c",
     "libavformat/hlsproto.c",
     "libavformat/hnm.c",
-    "libavformat/http.c",
+    // "libavformat/http.c",
     "libavformat/httpauth.c",
     "libavformat/iamf.c",
     "libavformat/iamf_parse.c",
@@ -6237,7 +6237,7 @@ const all_sources = [_][]const u8{
     "libavformat/mxfenc.c",
     "libavformat/mxg.c",
     "libavformat/ncdec.c",
-    "libavformat/network.c",
+    // "libavformat/network.c",
     "libavformat/nistspheredec.c",
     "libavformat/nspdec.c",
     "libavformat/nsvdec.c",
@@ -6357,14 +6357,14 @@ const all_sources = [_][]const u8{
     "libavformat/rtpenc_vp8.c",
     "libavformat/rtpenc_vp9.c",
     "libavformat/rtpenc_xiph.c",
-    "libavformat/rtpproto.c",
-    "libavformat/rtsp.c",
-    "libavformat/rtspdec.c",
-    "libavformat/rtspenc.c",
+    // "libavformtt/rtpproto.c",
+    // "libavformat/rtsp.c",
+    // "libavformat/rtspdec.c",
+    // "libavformat/rtspenc.c",
     "libavformat/s337m.c",
     "libavformat/samidec.c",
-    "libavformat/sapdec.c",
-    "libavformat/sapenc.c",
+    // "libavformat/sapdec.c",
+    // "libavformat/sapenc.c",
     "libavformat/sauce.c",
     "libavformat/sbcdec.c",
     "libavformat/sbgdec.c",
@@ -6401,7 +6401,7 @@ const all_sources = [_][]const u8{
     "libavformat/srtdec.c",
     "libavformat/srtenc.c",
     "libavformat/srtp.c",
-    "libavformat/srtpproto.c",
+    // "libavformat/srtpproto.c",
     "libavformat/stldec.c",
     "libavformat/subfile.c",
     "libavformat/subtitles.c",
@@ -6415,7 +6415,7 @@ const all_sources = [_][]const u8{
     "libavformat/swfdec.c",
     "libavformat/swfenc.c",
     "libavformat/takdec.c",
-    "libavformat/tcp.c",
+    // "libavformat/tcp.c",
     "libavformat/tedcaptionsdec.c",
     "libavformat/tee.c",
     "libavformat/tee_common.c",
@@ -6437,9 +6437,9 @@ const all_sources = [_][]const u8{
     "libavformat/tty.c",
     "libavformat/txd.c",
     "libavformat/ty.c",
-    "libavformat/udp.c",
+    // "libavformat/udp.c",
     "libavformat/uncodedframecrcenc.c",
-    "libavformat/unix.c",
+    // "libavformat/unix.c",
     "libavformat/url.c",
     "libavformat/urldecode.c",
     "libavformat/usmdec.c",
diff --git a/libavformat/demuxer_list.c b/libavformat/demuxer_list.c
index f31941ba50..3f5b670720 100644
--- a/libavformat/demuxer_list.c
+++ b/libavformat/demuxer_list.c
@@ -232,17 +232,13 @@ static const FFInputFormat * const demuxer_list[] = {
     &ff_rpl_demuxer,
     &ff_rsd_demuxer,
     &ff_rso_demuxer,
-    &ff_rtp_demuxer,
-    &ff_rtsp_demuxer,
     &ff_s337m_demuxer,
     &ff_sami_demuxer,
-    &ff_sap_demuxer,
     &ff_sbc_demuxer,
     &ff_sbg_demuxer,
     &ff_scc_demuxer,
     &ff_scd_demuxer,
     &ff_sdns_demuxer,
-    &ff_sdp_demuxer,
     &ff_sdr2_demuxer,
     &ff_sds_demuxer,
     &ff_sdx_demuxer,
diff --git a/libavformat/muxer_list.c b/libavformat/muxer_list.c
index c733f1887e..7a5f2acbe0 100644
--- a/libavformat/muxer_list.c
+++ b/libavformat/muxer_list.c
@@ -139,8 +139,6 @@ static const FFOutputFormat * const muxer_list[] = {
     &ff_rso_muxer,
     &ff_rtp_muxer,
     &ff_rtp_mpegts_muxer,
-    &ff_rtsp_muxer,
-    &ff_sap_muxer,
     &ff_sbc_muxer,
     &ff_scc_muxer,
     &ff_segafilm_muxer,
```

and finally build everything like so (assumes you clone all the dependencies into the ffmpeg dir):

```sh
#! /bin/sh

f() {
    zig build -Doptimize=ReleaseSmall -Dtarget=$1 -Dcpu=$2

    cd libmp3lame
    zig build -Doptimize=ReleaseFast -Dtarget=$1 -Dcpu=$2

    cd ../libogg
    zig build -Doptimize=ReleaseFast -Dtarget=$1 -Dcpu=$2

    cd ../libvorbis
    zig build -Doptimize=ReleaseFast -Dtarget=$1 -Dcpu=$2

    cd ../zlib
    zig build -Doptimize=ReleaseFast -Dtarget=$1 -Dcpu=$2

    cd ..
    cp zig-out/lib/libffmpeg.a blah/libffmpeg_$3.a
    cp libmp3lame/zig-out/lib/libmp3lame.a blah/libmp3lame_$3.a
    cp libogg/zig-out/lib/libogg.a blah/libogg_$3.a
    cp libvorbis/zig-out/lib/libvorbis.a blah/libvorbis_$3.a
    cp zlib/zig-out/lib/libz.a blah/libz_$3.a
}

mkdir -p blah
f x86_64-linux-musl x86_64_v3 linux_amd64
f aarch64-linux-musl baseline linux_arm64
f aarch64-macos apple_m1 darwin_arm64
```

ffmpeg `ReleaseSmall` enables specific size optimisations in ffmpeg that make it much smaller,
`ReleaseFast` builds are too big for GitHub...
