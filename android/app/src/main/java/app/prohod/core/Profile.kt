package app.prohod.core

import android.content.Context
import java.io.File

/**
 * Builds the engine argument vector from the bundled domain lists.
 * Mirrors the desktop core (internal/profile): pinned groups first, then a plain pass-through group,
 * then an automatic fallback chain. Method internals never surface in the UI.
 */
object Profile {
    private data class Method(val id: String, val label: String, val args: List<String>)

    private val methods = listOf(
        Method("S01", "A", listOf("-r1+s", "-o1")),
        Method("S02", "B", listOf("-s25", "-r5+s", "-s25+s")),
        Method("S03", "C", listOf("-o1")),
        Method("S04", "D", listOf("-o1", "-r-5+se")),
        Method("S05", "E", listOf("-d1")),
        Method("S06", "F", listOf("-o3", "-d7")),
        Method("S09", "G", listOf("-o2", "-s1", "-q1")),
        Method("S08", "H", listOf("-d1", "-s1+s", "-d3+s", "-s6+s", "-d9+s", "-s12+s", "-d15+s", "-s20+s", "-d25+s", "-s30+s", "-d35+s")),
        Method("S10", "I", listOf("-r1+s", "-d1")),
    ).associateBy { it.id }

    private val pinOrder = listOf("S01", "S04", "S02", "S03", "S05", "S06", "S09", "S08", "S10")
    private val chainOrder = listOf("S02", "S03", "S05", "S01", "S06", "S09")

    /** Writes the domain lists into listDir and returns engine args (without the listen address). */
    fun build(ctx: Context, listDir: File, ipv4Only: Boolean): List<String> {
        listDir.mkdirs()
        val args = mutableListOf("-W", "2", "-u", "86400")
        if (ipv4Only) args.add("-X")
        var first = true
        for (id in pinOrder) {
            val domains = readPins(ctx, id)
            if (domains.isEmpty()) continue
            val f = File(listDir, "l${id.substring(1)}.txt")
            f.writeText(domains.joinToString("\n") + "\n")
            f.setReadable(false, false); f.setReadable(true, true)
            if (!first) args.add("-An")
            args.add("-H"); args.add(f.absolutePath)
            args.addAll(methods.getValue(id).args)
            first = false
        }
        args.add("-An") // everything else: untouched first
        for (id in chainOrder) {
            args.add("-A"); args.add("t,s")
            args.addAll(methods.getValue(id).args)
        }
        return args
    }

    /** Number of domains covered by pinned groups (for the UI). */
    fun pinnedCount(ctx: Context): Int =
        pinOrder.sumOf { readPins(ctx, it).size }

    private fun readPins(ctx: Context, id: String): List<String> = try {
        ctx.assets.open("pins/$id.txt").bufferedReader().useLines { lines ->
            lines.map { it.trim().lowercase() }
                .filter { it.isNotEmpty() && !it.startsWith("#") }
                .distinct().toList()
        }
    } catch (_: Exception) {
        emptyList()
    }

    // --- service health mapping (shared with the UI) ---
    private val services = listOf(
        "Instagram" to listOf("instagram.com", "cdninstagram.com"),
        "YouTube" to listOf("youtube.com", "youtu.be", "googlevideo.com", "ytimg.com", "ggpht.com", "youtubei.googleapis.com"),
        "Facebook" to listOf("facebook.com", "fbcdn.net", "fb.com", "fb.me", "fbsbx.com", "messenger.com"),
        "X" to listOf("x.com", "twitter.com", "twimg.com", "t.co"),
        "Discord" to listOf("discord.com", "discord.gg", "discordapp.com", "discordapp.net", "discord.media"),
    )

    fun serviceOf(host: String): String? {
        val h = host.lowercase().trimEnd('.')
        for ((name, domains) in services)
            if (domains.any { h == it || h.endsWith(".$it") }) return name
        return null
    }
}
