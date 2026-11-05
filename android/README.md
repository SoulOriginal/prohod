# Prohod for Android

A VpnService app: TUN → tun2socks (libhev-socks5-tunnel) → the engine (libprohodcore.so) on loopback → internet.
Same routing knowledge as the desktop core (pinned domain lists + plain pass-through + auto fallback), bundled
as assets. No method internals are shown in the UI.

## Build
```
scripts/android-natives.sh                 # builds the native libs for arm64-v8a, armeabi-v7a, x86_64
cd android
JAVA_HOME=<jdk-17-or-21> ./gradlew :app:assembleRelease
# APK: android/app/build/outputs/apk/release/app-release.apk
```
Needs: Android SDK 34, NDK 27, a JDK 17–21 (Gradle 8.9 does not run on JDK 27).
The native `.so` files live under `android/app/src/main/jniLibs/` and are produced by the script, not committed.

## Notes
- `libprohodcore.so` is the engine executable, shipped as a lib so Android extracts it runnable
  (`extractNativeLibs=true`, `useLegacyPackaging=true`).
- The JNI class name `app.prohod.core.Tunnel` is fixed at native build time (-DPKGNAME/-DCLSNAME); don't rename it.
