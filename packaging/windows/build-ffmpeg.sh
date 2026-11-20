#!/bin/sh
# Builds the ffmpeg.exe that ships in the Windows release archives: an x64
# LGPL build with only what spareglass uses (gdigrab screen capture →
# Media Foundation H.264 → raw Annex-B on stdout). Cross-compiled on Linux
# with mingw-w64 (Debian/Ubuntu: apt-get install mingw-w64 make).
#
# Usage: packaging/windows/build-ffmpeg.sh OUT_DIR
# Writes OUT_DIR/ffmpeg.exe and OUT_DIR/ffmpeg-LICENSE.txt.
set -eu

FFMPEG_VERSION=9.1
FFMPEG_SHA256=1a4d394ae63d190a526cd052eb4415c156b5c5e2df37efab41a7de0b8fde2433

here=$(cd "$(dirname "$0")" && pwd)
out=$(mkdir -p "$1" && cd "$1" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"

curl -fsSL -o src.tar.xz "https://ffmpeg.org/releases/ffmpeg-$FFMPEG_VERSION.tar.xz"
echo "$FFMPEG_SHA256  src.tar.xz" | sha256sum -c -
tar xf src.tar.xz
src="$work/ffmpeg-$FFMPEG_VERSION"

mkdir build
cd build
printf '1 24 "%s"\n' "$here/ffmpeg.manifest" >manifest.rc
x86_64-w64-mingw32-windres manifest.rc -O coff -o manifest.o
"$src/configure" \
	--arch=x86_64 --target-os=mingw32 --cross-prefix=x86_64-w64-mingw32- \
	--enable-cross-compile --pkg-config=false \
	--extra-ldflags="-static -static-libgcc" --extra-libs="$PWD/manifest.o" \
	--disable-everything --disable-autodetect \
	--disable-doc --disable-debug --disable-network --disable-x86asm \
	--disable-ffplay --disable-ffprobe \
	--enable-w32threads --enable-mediafoundation \
	--enable-indev=gdigrab,lavfi \
	--enable-encoder=h264_mf \
	--enable-decoder=bmp,rawvideo,wrapped_avframe \
	--enable-muxer=h264 \
	--enable-protocol=pipe,file \
	--enable-bsf=dump_extradata,h264_mp4toannexb \
	--enable-filter=buffer,buffersink,abuffer,abuffersink,null,anull,format,aformat,scale,testsrc2 \
	>configure.log || { tail -40 configure.log; tail -60 ffbuild/config.log; exit 1; }
make -j"$(nproc)" ffmpeg.exe >make.log 2>&1 || { tail -60 make.log; exit 1; }
x86_64-w64-mingw32-strip ffmpeg.exe
cp ffmpeg.exe "$out/ffmpeg.exe"
cp "$src/COPYING.LGPLv2.1" "$out/ffmpeg-LICENSE.txt"
printf '\nffmpeg %s, source: https://ffmpeg.org/releases/ffmpeg-%s.tar.xz\nBuilt by packaging/windows/build-ffmpeg.sh in https://github.com/naoking158/spareglass\n' \
	"$FFMPEG_VERSION" "$FFMPEG_VERSION" >>"$out/ffmpeg-LICENSE.txt"
ls -la "$out"
