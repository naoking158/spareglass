#!/bin/sh
# Builds the ffmpeg that ships in the macOS release archive: a universal
# (arm64 + x86_64) LGPL build with only what spareglass uses
# (avfoundation screen capture → VideoToolbox H.264 → raw Annex-B on stdout).
#
# Usage: packaging/macos/build-ffmpeg.sh OUT_DIR
# Writes OUT_DIR/ffmpeg and OUT_DIR/ffmpeg-LICENSE.md.
set -eu

FFMPEG_VERSION=9.1
FFMPEG_SHA256=1a4d394ae63d190a526cd052eb4415c156b5c5e2df37efab41a7de0b8fde2433
MACOS_MIN=14.0

out=$(mkdir -p "$1" && cd "$1" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"

curl -fsSL -o src.tar.xz "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VERSION.tar.xz"
echo "$FFMPEG_SHA256  src.tar.xz" | shasum -a 256 -c -
tar xf src.tar.xz
src="$work/ffmpeg-$FFMPEG_VERSION"

build() {
	arch=$1
	mkdir "$work/$arch"
	cd "$work/$arch"
	"$src/configure" \
		--arch="$arch" --enable-cross-compile --target-os=darwin \
		--cc="clang -arch $arch" \
		--extra-cflags="-mmacosx-version-min=$MACOS_MIN" \
		--extra-ldflags="-mmacosx-version-min=$MACOS_MIN" \
		--disable-everything --disable-autodetect \
		--disable-doc --disable-debug --disable-network --disable-x86asm \
		--disable-ffplay --disable-ffprobe \
		--enable-avfoundation --enable-videotoolbox \
		--enable-indev=avfoundation,lavfi \
		--enable-encoder=h264_videotoolbox \
		--enable-muxer=h264 \
		--enable-protocol=pipe,file \
		--enable-bsf=dump_extradata,h264_mp4toannexb \
		--enable-decoder=rawvideo,wrapped_avframe \
		--enable-filter=buffer,buffersink,abuffer,abuffersink,null,anull,format,aformat,scale,testsrc2 \
		>configure.log
	make -j"$(sysctl -n hw.ncpu)" ffmpeg >make.log
	cd "$work"
}

build arm64
build x86_64
lipo -create -output "$out/ffmpeg" arm64/ffmpeg x86_64/ffmpeg
cp "$src/COPYING.LGPLv2.1" "$out/ffmpeg-LICENSE.md"
printf '\nffmpeg %s, source: https://ffmpeg.org/releases/ffmpeg-%s.tar.xz\nBuilt by packaging/macos/build-ffmpeg.sh in https://github.com/naoking158/spareglass\n' \
	"$FFMPEG_VERSION" "$FFMPEG_VERSION" >>"$out/ffmpeg-LICENSE.md"
"$out/ffmpeg" -hide_banner -version | head -1
