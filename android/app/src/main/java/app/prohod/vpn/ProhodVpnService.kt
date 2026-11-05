package app.prohod.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat
import app.prohod.MainActivity
import app.prohod.R
import app.prohod.core.Engine
import app.prohod.core.Tunnel
import java.io.File
import java.util.concurrent.Executors

/**
 * The connection: a TUN whose packets go to tun2socks → the engine (loopback) → the internet.
 * State is exposed through a static flow the UI observes; nothing about the method is shown.
 *
 * All socket/engine work runs on a dedicated single-thread executor so the main thread never
 * touches the network (which would throw NetworkOnMainThreadException). startForeground() is
 * always called on the main thread within onStartCommand so the OS never ANRs the service.
 */
class ProhodVpnService : VpnService() {

    private var tun: ParcelFileDescriptor? = null
    private var engine: Engine? = null
    private val worker = Executors.newSingleThreadExecutor()
    @Volatile private var shuttingDown = false

    override fun onCreate() {
        super.onCreate()
        ensureChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // Must enter the foreground promptly on every start, on the main thread.
        startForegroundCompat()

        if (intent?.action == ACTION_STOP) {
            State.set(State.STOPPING, null)
            submit { stopEverything() }
            return START_NOT_STICKY
        }

        State.set(State.CONNECTING, null)
        submit {
            try {
                connect()
                State.set(State.ON, null)
                Log.i(TAG, "connected")
            } catch (e: Throwable) {
                Log.e(TAG, "connect failed", e)
                teardown()
                State.set(State.OFF, "Не удалось подключиться")
                stopSelfSafely()
            }
        }
        return START_STICKY
    }

    private fun connect() {
        val eng = Engine(this)
        val port = eng.start(ipv4Only = Prefs.ipv4Only(this))
        engine = eng

        val builder = Builder()
            .setSession("Prohod")
            .setMtu(MTU)
            .addAddress("10.25.0.2", 32)
            .addDnsServer("1.1.1.1")
            .addDnsServer("8.8.8.8")
            .addRoute("0.0.0.0", 0)
        if (!Prefs.ipv4Only(this)) {
            try {
                builder.addAddress("fd00:25::2", 128)
                builder.addRoute("::", 0)
            } catch (_: Exception) {}
        }
        // keep our own traffic (and the engine's upstream) out of the tunnel
        try { builder.addDisallowedApplication(packageName) } catch (_: Exception) {}
        builder.setConfigureIntent(
            PendingIntent.getActivity(
                this, 0, Intent(this, MainActivity::class.java),
                PendingIntent.FLAG_IMMUTABLE
            )
        )
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) builder.setMetered(false)

        val fd = builder.establish() ?: throw IllegalStateException("VPN permission missing")
        tun = fd

        val cfg = File(filesDir, "tun2socks.yml")
        cfg.writeText(
            """
            tunnel:
              mtu: $MTU
            misc:
              task-stack-size: 81920
              log-level: none
            socks5:
              address: 127.0.0.1
              port: $port
              udp: 'udp'
            """.trimIndent()
        )
        Tunnel.TProxyStartService(cfg.absolutePath, fd.fd)
    }

    /** Stops the data plane without touching the foreground/service lifecycle. */
    private fun teardown() {
        try { Tunnel.TProxyStopService() } catch (_: Throwable) {}
        engine?.stop(); engine = null
        try { tun?.close() } catch (_: Exception) {}
        tun = null
    }

    /** Full stop requested by the user or the system: tear down and leave the foreground. */
    private fun stopEverything() {
        if (shuttingDown) return
        shuttingDown = true
        teardown()
        State.set(State.OFF, null)
        stopSelfSafely()
    }

    private fun stopSelfSafely() {
        try { stopForeground(STOP_FOREGROUND_REMOVE) } catch (_: Exception) {}
        stopSelf()
    }

    override fun onDestroy() {
        submit { teardown() }
        worker.shutdown()
        if (State.value != State.OFF) State.set(State.OFF, null)
        super.onDestroy()
    }

    override fun onRevoke() {
        State.set(State.STOPPING, null)
        submit { stopEverything() }
        super.onRevoke()
    }

    /** Runs work on the serial worker; never throws if the executor is already shutting down. */
    private fun submit(block: () -> Unit) {
        try {
            worker.execute(block)
        } catch (_: java.util.concurrent.RejectedExecutionException) {
            try { block() } catch (_: Throwable) {}
        }
    }

    private fun startForegroundCompat() {
        val n = buildNotification()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(NOTIF_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIF_ID, n)
        }
    }

    private fun ensureChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val mgr = getSystemService(NotificationManager::class.java)
            val ch = NotificationChannel(CHANNEL, "Prohod", NotificationManager.IMPORTANCE_LOW)
            ch.setShowBadge(false)
            mgr.createNotificationChannel(ch)
        }
    }

    private fun buildNotification(): Notification {
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )
        val stop = PendingIntent.getService(
            this, 1, Intent(this, ProhodVpnService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )
        return NotificationCompat.Builder(this, CHANNEL)
            .setContentTitle("Prohod")
            .setContentText("Защита включена")
            .setSmallIcon(R.drawable.ic_stat)
            .setOngoing(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setContentIntent(open)
            .addAction(0, "Отключить", stop)
            .build()
    }

    companion object {
        const val ACTION_STOP = "app.prohod.STOP"
        private const val CHANNEL = "prohod"
        private const val NOTIF_ID = 1
        private const val MTU = 8500
        private const val TAG = "prohod"

        fun start(ctx: Context) {
            val i = Intent(ctx, ProhodVpnService::class.java)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) ctx.startForegroundService(i) else ctx.startService(i)
        }

        fun stop(ctx: Context) {
            val i = Intent(ctx, ProhodVpnService::class.java).setAction(ACTION_STOP)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) ctx.startForegroundService(i) else ctx.startService(i)
        }
    }
}
