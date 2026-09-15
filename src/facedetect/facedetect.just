facedetect: (_facedetect "x86_64-linux-musl" "x86_64_v3" "linux_amd64") \
		(_facedetect "aarch64-linux-musl" "baseline" "linux_arm64") \
		(_facedetect "aarch64-macos" "apple_m1" "darwin_arm64")

_facedetect target cpu suffix:
	cd src/facedetect && zig build -Doptimize=ReleaseFast -Dtarget={{target}} -Dcpu={{cpu}}
	mv src/facedetect/zig-out/lib/libfacedetect.a src/facedetect/libfacedetect_{{suffix}}.a
