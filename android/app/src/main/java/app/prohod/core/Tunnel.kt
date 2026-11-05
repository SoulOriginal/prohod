package app.prohod.core

/**
 * JNI bridge to libhev-socks5-tunnel.so (tun2socks). The native class name is fixed at build time
 * (-DPKGNAME=app/prohod/core -DCLSNAME=Tunnel), so this package/class name must not change.
 */
object Tunnel {
    init { System.loadLibrary("hev-socks5-tunnel") }

    /** Starts tun2socks: reads the TUN [fd], relays into the SOCKS5 server described by the YAML at [configPath]. */
    external fun TProxyStartService(configPath: String, fd: Int)

    external fun TProxyStopService()

    /** Returns [txPackets, txBytes, rxPackets, rxBytes] or null. */
    external fun TProxyGetStats(): LongArray?
}
