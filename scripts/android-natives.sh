#!/bin/bash
# Build Android native parts into android/app/src/main/jniLibs/<abi>/ .
# The engine is an executable; Android only installs lib*.so files, so it is shipped as libprohodcore.so.
set -euo pipefail
cd "$(dirname "$0")/../android/native"
NDK=${ANDROID_NDK:-$HOME/Android/Sdk/ndk/27.2.12479018}
OUT=../app/src/main/jniLibs
"$NDK/ndk-build" NDK_PROJECT_PATH=. APP_BUILD_SCRIPT=jni/Android.mk NDK_APPLICATION_MK=jni/Application.mk \
  NDK_LIBS_OUT="$OUT" -j"$(nproc)" >/dev/null
for d in "$OUT"/*/; do
  [ -f "$d/libprohodcore" ] && mv -f "$d/libprohodcore" "$d/libprohodcore.so"
done
find "$OUT" -type f -name '*.so' | sort
