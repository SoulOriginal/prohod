package app.prohod.vpn

import android.content.Context

/** Connection state, observed by the UI. Deliberately coarse — no method details. */
object State {
    const val OFF = "off"
    const val CONNECTING = "connecting"
    const val ON = "on"
    const val STOPPING = "stopping"

    @Volatile var value: String = OFF; private set
    @Volatile var message: String? = null; private set
    @Volatile var since: Long = 0; private set

    private val listeners = mutableListOf<() -> Unit>()

    @Synchronized
    fun set(state: String, msg: String?) {
        if (state != value) since = System.currentTimeMillis()
        value = state
        message = msg
        listeners.toList().forEach { it() }
    }

    @Synchronized fun observe(cb: () -> Unit) { listeners.add(cb) }
    @Synchronized fun unobserve(cb: () -> Unit) { listeners.remove(cb) }
}

/** Minimal settings store (no secrets). */
object Prefs {
    private fun sp(ctx: Context) = ctx.getSharedPreferences("prohod", Context.MODE_PRIVATE)
    fun ipv4Only(ctx: Context): Boolean = sp(ctx).getBoolean("ipv4Only", false)
    fun setIpv4Only(ctx: Context, v: Boolean) = sp(ctx).edit().putBoolean("ipv4Only", v).apply()
}
