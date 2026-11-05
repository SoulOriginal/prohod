package app.prohod.core

import android.content.Context
import android.util.Log
import java.io.File
import java.net.InetSocketAddress
import java.net.Socket

/**
 * Runs the embedded engine (libprohodcore.so, shipped as a native lib so Android extracts it executable)
 * on a loopback port with the generated profile. Never logs the arguments.
 */
class Engine(private val ctx: Context) {
    @Volatile private var process: Process? = null
    @Volatile var port: Int = 0; private set

    val isRunning: Boolean get() = process?.isAlive == true

    /** Starts the engine and waits until it accepts connections. Returns the loopback port. */
    fun start(ipv4Only: Boolean): Int {
        stop()
        val bin = File(ctx.applicationInfo.nativeLibraryDir, "libprohodcore.so")
        if (!bin.exists()) throw IllegalStateException("core binary missing")
        val p = freePort()
        val listDir = File(ctx.filesDir, "lists")
        val args = Profile.build(ctx, listDir, ipv4Only)
        val cmd = mutableListOf(bin.absolutePath, "-i", "127.0.0.1", "-p", p.toString())
        cmd.addAll(args)
        val proc = ProcessBuilder(cmd)
            .directory(ctx.filesDir)
            .redirectErrorStream(true)
            .start()
        process = proc
        port = p
        // drain output so the pipe never blocks; keep nothing.
        // Must swallow everything: when the process is destroyed the blocked read is
        // interrupted and throws, which would otherwise crash the whole app process.
        Thread {
            try {
                proc.inputStream.bufferedReader().forEachLine { /* discard */ }
            } catch (_: Throwable) { /* process gone / stream closed */ }
        }.apply { isDaemon = true }.start()
        val deadline = System.currentTimeMillis() + 4000
        while (System.currentTimeMillis() < deadline) {
            if (!proc.isAlive) throw IllegalStateException("engine exited at start")
            if (accepts(p)) return p
            Thread.sleep(80)
        }
        stop()
        throw IllegalStateException("engine did not start listening")
    }

    fun stop() {
        process?.let {
            it.destroy()
            if (!it.waitForQuietly(800)) it.destroyForcibly()
        }
        process = null
        port = 0
    }

    private fun accepts(p: Int): Boolean = try {
        Socket().use { it.connect(InetSocketAddress("127.0.0.1", p), 200); true }
    } catch (_: Exception) { false }

    private fun freePort(): Int = java.net.ServerSocket(0, 1, java.net.InetAddress.getByName("127.0.0.1")).use { it.localPort }

    private fun Process.waitForQuietly(ms: Long): Boolean = try {
        waitFor(ms, java.util.concurrent.TimeUnit.MILLISECONDS)
    } catch (_: InterruptedException) { false }

    companion object { private const val TAG = "prohod" }
}
