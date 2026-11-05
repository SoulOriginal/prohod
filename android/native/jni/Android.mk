# Native parts of the Android app:
#   libhev-socks5-tunnel.so  tun -> local SOCKS5 (JNI, class app.prohod.core.Tunnel)   third_party/hev-socks5-tunnel (MIT)
#   libprohodcore.so         the engine as an executable (run from nativeLibraryDir)    third_party/byedpi (MIT)
NATIVE_ROOT := $(call my-dir)/../../../third_party

include $(NATIVE_ROOT)/hev-socks5-tunnel/Android.mk

include $(CLEAR_VARS)
LOCAL_PATH            := $(NATIVE_ROOT)/byedpi
LOCAL_MODULE          := prohodcore
LOCAL_MODULE_FILENAME := libprohodcore
LOCAL_SRC_FILES       := packets.c main.c conev.c proxy.c desync.c mpool.c extend.c
LOCAL_CFLAGS          := -std=c99 -O2 -D_DEFAULT_SOURCE -Wno-unused -Wno-unused-parameter
include $(BUILD_EXECUTABLE)
