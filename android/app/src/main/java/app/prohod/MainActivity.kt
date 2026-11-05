package app.prohod

import android.app.Activity
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.ViewGroup
import android.widget.FrameLayout
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.core.widget.ImageViewCompat
import androidx.core.content.res.ResourcesCompat
import android.content.res.ColorStateList
import app.prohod.core.Profile
import app.prohod.vpn.Prefs
import app.prohod.vpn.ProhodVpnService
import app.prohod.vpn.State
import com.google.android.material.materialswitch.MaterialSwitch

/** Single-screen UI: a big power button, status, a session timer, service health and one setting.
 *  No method internals are ever shown. */
class MainActivity : AppCompatActivity() {

    private lateinit var powerButton: FrameLayout
    private lateinit var powerIcon: ImageView
    private lateinit var statusTitle: TextView
    private lateinit var statusSub: TextView
    private lateinit var routesInfo: TextView
    private lateinit var ipv4Switch: MaterialSwitch
    private val serviceDots = mutableListOf<ImageView>()

    private val ui = Handler(Looper.getMainLooper())
    private val repaint: () -> Unit = { ui.post { render() } }

    private val services = listOf("Instagram", "YouTube", "X", "Discord", "Facebook")

    private val prepare = registerForActivityResult(
        androidx.activity.result.contract.ActivityResultContracts.StartActivityForResult()
    ) { if (it.resultCode == Activity.RESULT_OK) ProhodVpnService.start(this) }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 0)
        }
        setContentView(R.layout.activity_main)

        powerButton = findViewById(R.id.powerButton)
        powerIcon = findViewById(R.id.powerIcon)
        statusTitle = findViewById(R.id.statusTitle)
        statusSub = findViewById(R.id.statusSub)
        routesInfo = findViewById(R.id.routesInfo)
        ipv4Switch = findViewById(R.id.ipv4Switch)

        powerButton.setOnClickListener { toggle() }

        ipv4Switch.isChecked = Prefs.ipv4Only(this)
        ipv4Switch.setOnCheckedChangeListener { _, checked -> Prefs.setIpv4Only(this, checked) }

        buildServiceChips()
        routesInfo.text = "Оптимизировано маршрутов: ${Profile.pinnedCount(this)}"
    }

    private fun buildServiceChips() {
        val container = findViewById<LinearLayout>(R.id.services)
        val density = resources.displayMetrics.density
        for (name in services) {
            val col = LinearLayout(this).apply {
                orientation = LinearLayout.VERTICAL
                gravity = Gravity.CENTER
                layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f)
            }
            val dot = ImageView(this).apply {
                setImageDrawable(ResourcesCompat.getDrawable(resources, R.drawable.dot, theme))
                val s = (8 * density).toInt()
                layoutParams = LinearLayout.LayoutParams(s, s)
                ImageViewCompat.setImageTintList(this, ColorStateList.valueOf(dotColor()))
            }
            val label = TextView(this).apply {
                text = name
                setTextColor(ContextCompat.getColor(this@MainActivity, R.color.text_secondary))
                textSize = 11f
                gravity = Gravity.CENTER
                val lp = LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT
                )
                lp.topMargin = (6 * density).toInt()
                layoutParams = lp
            }
            serviceDots.add(dot)
            col.addView(dot)
            col.addView(label)
            container.addView(col)
        }
    }

    override fun onStart() {
        super.onStart()
        State.observe(repaint)
        render()
        tick()
    }

    override fun onStop() {
        super.onStop()
        State.unobserve(repaint)
        ui.removeCallbacksAndMessages(null)
    }

    private fun tick() {
        if (State.value == State.ON) render()
        ui.postDelayed({ tick() }, 1000)
    }

    private fun toggle() {
        when (State.value) {
            State.ON -> ProhodVpnService.stop(this)
            State.OFF -> {
                val intent = VpnService.prepare(this)
                if (intent != null) prepare.launch(intent) else ProhodVpnService.start(this)
            }
            // CONNECTING / STOPPING: ignore taps mid-transition
        }
    }

    private fun dotColor(): Int = when (State.value) {
        State.ON -> ContextCompat.getColor(this, R.color.accent_on)
        else -> ContextCompat.getColor(this, R.color.dot_off)
    }

    private fun render() {
        val on = State.value == State.ON
        when (State.value) {
            State.ON -> {
                statusTitle.text = "Защищено"
                statusSub.text = elapsed()
                powerButton.setBackgroundResource(R.drawable.power_ring_on)
                tintIcon(R.color.accent_on)
            }
            State.CONNECTING -> {
                statusTitle.text = "Подключение"
                statusSub.text = "Устанавливаем защищённое соединение…"
                powerButton.setBackgroundResource(R.drawable.power_ring_connecting)
                tintIcon(R.color.accent_connecting)
            }
            State.STOPPING -> {
                statusTitle.text = "Отключение"
                statusSub.text = "…"
                powerButton.setBackgroundResource(R.drawable.power_ring_connecting)
                tintIcon(R.color.accent_connecting)
            }
            else -> {
                statusTitle.text = "Не подключено"
                statusSub.text = State.message ?: "Нажмите, чтобы включить"
                powerButton.setBackgroundResource(R.drawable.power_ring_off)
                tintIcon(R.color.accent_off)
            }
        }
        // mid-transition the button should not accept taps
        powerButton.isClickable = State.value == State.ON || State.value == State.OFF
        ipv4Switch.isEnabled = State.value == State.OFF

        val dc = dotColor()
        for (d in serviceDots) ImageViewCompat.setImageTintList(d, ColorStateList.valueOf(dc))
    }

    private fun tintIcon(colorRes: Int) {
        ImageViewCompat.setImageTintList(
            powerIcon, ColorStateList.valueOf(ContextCompat.getColor(this, colorRes))
        )
    }

    private fun elapsed(): String {
        val s = ((System.currentTimeMillis() - State.since) / 1000).coerceAtLeast(0)
        val h = s / 3600; val m = (s / 60) % 60; val ss = s % 60
        return if (h > 0) "%d:%02d:%02d".format(h, m, ss) else "%02d:%02d".format(m, ss)
    }
}
