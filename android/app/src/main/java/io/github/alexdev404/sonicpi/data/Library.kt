// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.data

import android.content.res.AssetManager
import org.json.JSONArray
import org.json.JSONObject

/** One thing the Learn screen lists: an example program, a function, a synth, an FX or a sample group. */
data class LibraryItem(
    val key: String,
    val title: String,
    val summary: String,
    /** Sonic Pi's doc, as HTML (the reference) or plain text. */
    val docHtml: String = "",
    val usage: String = "",
    /** Code to show and open in a buffer: an example program, or a function's examples. */
    val code: List<String> = emptyList(),
    /** A synth's or FX's opts: name, default, what it does. */
    val opts: List<Triple<String, String, String>> = emptyList(),
    /** A sample group's samples. */
    val samples: List<String> = emptyList(),
)

data class LibrarySection(val title: String, val items: List<LibraryItem>)

/** Sonic Pi's examples and reference, from the assets the build copied out of the checkout. */
class Library(private val assets: AssetManager) {
    val sections: List<LibrarySection> by lazy {
        listOf(
            LibrarySection("Examples", examples()),
            LibrarySection("Functions", functions()),
            LibrarySection("Synths", instruments("synths")),
            LibrarySection("FX", instruments("fx")),
            LibrarySection("Samples", samples()),
        )
    }

    /** Every function name, for the editor's highlighting. */
    val functionNames: Set<String> by lazy { functions().map { it.key }.toSet() }

    private fun json(name: String) = JSONObject(assets.open("sonicpi/reference/$name.json").bufferedReader().readText())

    private fun examples(): List<LibraryItem> {
        val levels = listOf("apprentice", "illusionist", "magician", "sorcerer", "wizard", "algomancer")
        return levels.flatMap { level ->
            (assets.list("sonicpi/examples/$level") ?: emptyArray()).sorted().map { file ->
                val code = assets.open("sonicpi/examples/$level/$file").bufferedReader().readText()
                val key = file.removeSuffix(".rb")
                LibraryItem(
                    key = "$level/$key",
                    title = key.split('_').joinToString(" ") { w -> w.replaceFirstChar(Char::titlecase) },
                    summary = level.replaceFirstChar(Char::titlecase),
                    code = listOf(code),
                )
            }
        }
    }

    private fun functions(): List<LibraryItem> = json("lang").getJSONArray("pages").objects().map {
        LibraryItem(
            key = it.getString("key"),
            title = it.getString("key"),
            summary = it.optString("summary"),
            docHtml = it.optString("doc_html"),
            usage = it.optString("usage"),
            code = it.optJSONArray("examples")?.objects()?.map { e -> e.getString("code") }.orEmpty(),
        )
    }

    private fun instruments(name: String): List<LibraryItem> = json(name).getJSONArray("pages").objects().map {
        val key = it.getString("key")
        LibraryItem(
            key = key,
            title = it.optString("title", key),
            summary = it.optString("summary"),
            docHtml = it.optString("doc_html"),
            usage = if (name == "fx") "with_fx :$key do\n  play 60\nend" else "use_synth :$key\nplay 60",
            opts = it.optJSONArray("opts")?.objects()?.map { o ->
                Triple(o.getString("name"), o.opt("default")?.toString().orEmpty(), o.optString("doc"))
            }.orEmpty(),
        )
    }

    private fun samples(): List<LibraryItem> = json("samples").getJSONArray("groups").objects().map {
        val names = (0 until it.getJSONArray("samples").length()).map { i -> it.getJSONArray("samples").getString(i) }
        LibraryItem(key = it.getString("title"), title = it.getString("title"), summary = "${names.size} samples", samples = names)
    }

    private fun JSONArray.objects(): List<JSONObject> = (0 until length()).map { getJSONObject(it) }
}
