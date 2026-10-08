// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.editor.completion

import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import org.json.JSONArray
import org.json.JSONObject

// What to offer at the caret: a port of Sonic Pi's
// app/web/app/src/completion/api.js (itself a port of the desktop GUI's
// app/gui/utils/scintilla_api.cpp) and of the completion source in its cm.js,
// fed by the same data: app/web/web/data/completion.json, generated from the
// language, and the reference pages' name lists.

enum class Kind(val icon: String, val label: String) {
    Fn("λ", "function"), Opt(":", "opt"), OptVal("=", "value"), Note("♪", "note"),
    Synth("∿", "synth"), Fx("≈", "fx"), Sample("▸", "sample"), Chord("♫", "chord"), Scale("♫", "scale"),
    Tuning("♮", "tuning"), Example("✎", "example"), Port("⇄", "MIDI port"), Cue("⚑", "cue"),
}

/** One thing offered: what goes in, what it is, and its doc (HTML, as Sonic Pi's reference has it). */
data class Suggestion(val text: String, val kind: Kind, val summary: String = "", val usage: String = "", val doc: String = "")

/** What to offer at a caret, and the span of text a choice replaces. */
data class Suggestions(val from: Int, val to: Int, val items: List<Suggestion>)

/** What the word under the caret is: its name, a line saying what, how it is used, its doc. */
data class WordInfo(val title: String, val kind: Kind, val summary: String, val usage: String, val doc: String)

internal data class Entry(val summary: String, val usage: String, val doc: String)

/** completion.json, and the reference's names, read once. */
class CompletionData internal constructor(
    internal val synthArgs: Map<String, List<String>>,
    internal val fxArgs: Map<String, List<String>>,
    internal val playArgs: List<String>,
    internal val sampleArgs: List<String>,
    private val entries: Map<String, Entry>,
    internal val argKinds: Map<String, List<String>>,
    internal val fnOpts: Map<String, List<String>>,
    internal val optOptions: Map<String, List<String>>,
    internal val optRanges: Set<String>,
    private val ownerDocs: Map<String, String>,
    private val ownerOptions: Map<String, List<String>>,
    internal val ownerRanges: Set<String>,
    internal val chordIntervals: Map<String, List<Int>>,
    internal val scaleIntervals: Map<String, List<Int>>,
    internal val functions: List<String>,
    internal val synths: List<String>,
    internal val fx: List<String>,
    internal val samples: List<String>,
) {
    internal fun entry(name: String): Entry? = entries[name]
    internal fun summary(name: String) = entries[name]?.summary.orEmpty()
    internal fun usage(name: String) = entries[name]?.usage.orEmpty()
    internal fun ownerDoc(owner: String, opt: String, fallback: String) =
        (if (owner.isNotEmpty()) ownerDocs["$owner $opt"] else null) ?: fallback
    internal fun ownerOpts(owner: String, opt: String, fallback: List<String>) =
        (if (owner.isNotEmpty()) ownerOptions["$owner $opt"] else null) ?: fallback

    companion object {
        /**
         * [completion] is completion.json; [lang], [synths], [fx] and [samples]
         * the reference pages (app/web/web/data/reference/).
         */
        fun parse(completion: String, lang: String, synths: String, fx: String, samples: String): CompletionData {
            val c = JSONObject(completion)
            fun JSONArray.strings() = List(length()) { getString(it) }
            fun JSONArray.ints() = List(length()) { getInt(it) }
            fun JSONArray.objects() = List(length()) { getJSONObject(it) }
            fun <T> JSONObject.map(f: (JSONObject, String) -> T): Map<String, T> = keys().asSequence().associateWith { f(this, it) }
            fun stringLists(name: String) = c.getJSONObject(name).map { o, k -> o.getJSONArray(k).strings() }
            fun intLists(name: String) = c.getJSONObject(name).map { o, k -> o.getJSONArray(k).ints() }
            val owners = c.getJSONObject("optOwners")
            fun pages(json: String) = JSONObject(json).getJSONArray("pages").objects().map { it.getString("key") }
            return CompletionData(
                synthArgs = stringLists("synths"),
                fxArgs = stringLists("fx"),
                playArgs = c.getJSONArray("playArgs").strings(),
                sampleArgs = c.getJSONArray("sampleArgs").strings(),
                entries = c.getJSONObject("entries").map { o, k ->
                    o.getJSONObject(k).let { Entry(it.optString("summary"), it.optString("usage"), it.optString("doc")) }
                },
                argKinds = stringLists("argKinds"),
                fnOpts = stringLists("fnOpts"),
                optOptions = stringLists("optOptions"),
                optRanges = c.getJSONObject("optRanges").keys().asSequence().toSet(),
                ownerDocs = owners.getJSONArray("docs").objects().associate { "${it.getString("owner")} ${it.getString("opt")}" to it.getString("doc") },
                ownerOptions = owners.getJSONArray("options").objects()
                    .associate { "${it.getString("owner")} ${it.getString("opt")}" to it.getJSONArray("options").strings() },
                ownerRanges = owners.getJSONArray("ranges").objects().map { "${it.getString("owner")} ${it.getString("opt")}" }.toSet(),
                chordIntervals = intLists("chordIntervals"),
                scaleIntervals = intLists("scaleIntervals"),
                functions = pages(lang),
                synths = pages(synths).map { ":$it" },
                fx = pages(fx).map { ":$it" },
                samples = JSONObject(samples).getJSONArray("groups").objects()
                    .flatMap { g -> g.getJSONArray("samples").strings().map { ":$it" } },
            )
        }
    }
}

// Native keeps these lists by hand; so does api.js, and so does this.
private val CHORDS = listOf("'1'", "'5'", "'+5'", "'m+5'", ":sus2", ":sus4", "'6'", ":m6", "'7sus2'", "'7sus4'", "'7-5'", ":halfdiminished", "'7+5'", "'m7+5'", "'9'", ":m9", "'m7+9'", ":maj9", "'9sus4'", "'9-5'", "'6*9'", "'m6*9'", "'7-9'", "'m7-9'", "'7-10'", "'7+9'", "'7-11'", "'7-13'", "'9+5'", "'m9+5'", "'7+5-9'", "'m7+5-9'", "'11'", ":m11", ":maj11", "'11+'", "'m11+'", "'13'", ":m13", ":maj13", ":add2", ":add4", ":add9", ":add11", ":add13", ":madd2", ":madd4", ":madd9", ":madd11", ":madd13", ":major", ":maj", ":M", ":minor", ":min", ":m", ":major7", ":maj7", ":dom7", "'7'", ":M7", ":minor7", ":min7", ":m7", ":minor_major7", "'mM7'", "'mmaj7'", ":augmented", ":a", ":diminished", ":dim", ":i", ":diminished7", ":dim7", ":i7", ":halfdim", "'m7b5'", "'m7-5'")
private val EXAMPLES = listOf(":haunted", ":ambient_experiment", ":chord_inversions", ":filtered_dnb", ":fm_noise", ":jungle", ":ocean", ":reich_phase", ":acid", ":ambient", ":compus_beats", ":echo_drama", ":idm_breakbeat", ":tron_bike", ":wob_rhyth", ":bach", ":driving_pulse", ":monday_blues", ":rerezzed", ":square_skit", ":blimp_zones", ":blip_rhythm", ":shufflit", ":tilburg_2", ":time_machine", ":sonic_dreams", ":blockgame", ":cloud_beat", ":lorezzed")
private val TUNINGS = listOf(":just", ":pythagorean", ":meantone", ":equal")
private val MIDI_PARAMS = listOf("sustain:", "velocity:", "vel:", "velocity_f:", "vel_f:", "port:", "channel:")
private val RANDOM_SOURCES = listOf(":white", ":light_pink", ":pink", ":dark_pink", ":perlin")

/** MIDI numbers first, each with its note name; then the names, each with its number. C4 = 60. */
internal val NOTES: List<Suggestion> by lazy {
    val canon = listOf("c", "cs", "d", "eb", "e", "f", "fs", "g", "ab", "a", "bb", "b")
    val out = mutableListOf<Suggestion>()
    for (midi in 36..96) out += Suggestion("$midi", Kind.Note, ":${canon[midi % 12]}${midi / 12 - 1}")
    val spellings = listOf("c" to 0, "cs" to 1, "db" to 1, "d" to 2, "ds" to 3, "eb" to 3, "e" to 4, "f" to 5, "fs" to 6, "gb" to 6,
        "g" to 7, "gs" to 8, "ab" to 8, "a" to 9, "as" to 10, "bb" to 10, "b" to 11, "cb" to -1, "es" to 5, "fb" to 4, "bs" to 12)
    for (oct in 2..7) for ((name, offset) in spellings) {
        val midi = (oct + 1) * 12 + offset
        if (midi in 36..96) out += Suggestion(":$name$oct", Kind.Note, "$midi")
    }
    out
}

/** An opt's doc as a line: "Time for reverberation to complete in seconds, default 3". */
internal fun optSummary(html: String): String {
    if (html.isEmpty()) return ""
    fun text(h: String) = h.replace(Regex("<[^>]*>"), "").replace("&lt;", "<").replace("&gt;", ">").replace("&quot;", "\"")
        .replace("&#39;", "'").replace("&amp;", "&").replace(Regex("\\s+"), " ").trim()
    val paras = Regex("<p>([\\s\\S]*?)</p>").findAll(html).map { it.groupValues[1] }.toList()
    val def = paras.firstNotNullOfOrNull { Regex("^\\s*Default:\\s*([\\s\\S]*)$").find(it)?.groupValues?.get(1) }
    val say = paras.map(::text).firstOrNull { it.isNotEmpty() && !it.startsWith("Default:") }.orEmpty()
    val first = (Regex("^(.+?[.!?])(\\s|$)").find(say)?.groupValues?.get(1) ?: say).removeSuffix(".")
    val value = def?.let { text(Regex("<code>([\\s\\S]*?)</code>").find(it)?.groupValues?.get(1) ?: it.substringBefore("·")) }.orEmpty()
    return listOf(first, if (value.isNotEmpty() && !Regex("\\bdefault\\b", RegexOption.IGNORE_CASE).containsMatchIn(first)) "default $value" else "")
        .filter { it.isNotEmpty() }.joinToString(", ")
}

private fun lastWordBeforePartial(context: List<String>) = context.dropLast(1).lastOrNull { it.isNotEmpty() }.orEmpty()

/** The synth in effect at [pos]: the last use_synth or with_synth above it. */
fun synthAt(code: String, pos: Int): String =
    Regex("\\b(?:use_synth|with_synth)\\s*\\(?\\s*:(\\w+)").findAll(code.substring(0, pos.coerceIn(0, code.length))).lastOrNull()?.groupValues?.get(1).orEmpty()

class CompletionEngine(private val data: CompletionData) {
    private val keywords: Map<String, List<String>> = mapOf(
        "Func" to data.functions, "Synth" to data.synths, "FX" to data.fx, "Sample" to data.samples,
        "Chord" to CHORDS, "Scale" to data.scaleIntervals.keys.map { ":$it" }, "Examples" to EXAMPLES, "Tuning" to TUNINGS,
        "MidiParam" to MIDI_PARAMS, "RandomSource" to RANDOM_SOURCES, "PlayParam" to data.playArgs, "SampleParam" to data.sampleArgs,
        "CuePath" to emptyList(), "MidiOuts" to emptyList(),
    )

    private fun isNoteContext(context: List<String>): Boolean {
        val lw = lastWordBeforePartial(context)
        return lw == "play" || lw == "scale" || lw == "chord" || lw == "note:" || resolveArgKind(context, data.argKinds) == "Note"
    }

    private fun ownerForContext(context: List<String>, synth: String): String {
        val first = context.getOrElse(0) { "" }
        val second = context.getOrElse(1) { "" }
        return when {
            first == "with_fx" && second in data.fxArgs -> second
            first == "synth" && second in data.synthArgs -> second
            (first == "play" || first == "control") && synth.isNotEmpty() -> ":$synth"
            else -> ""
        }
    }

    /** api.js's completionsFor: the candidates for a context, unfiltered. */
    internal fun completionsFor(context: List<String>, synth: String): List<Suggestion> {
        if (isNoteContext(context)) return NOTES
        val optBefore = lastWordBeforePartial(context)
        val owner = ownerForContext(context, synth)
        data.optOptions[optBefore]?.let { values ->
            val doc = data.ownerDoc(owner, optBefore, data.entry(optBefore)?.doc.orEmpty())
            return data.ownerOpts(owner, optBefore, values).map { Suggestion(it, Kind.OptVal, enumLabelFor(doc, it), doc = doc) }
        }
        // A number's range: native's slider, which is only ever asked for (Tab), never offered.
        if ("$owner $optBefore" in data.ownerRanges || optBefore in data.optRanges) return emptyList()
        val (kind, names) = names(context, synth)
        return names.map { n ->
            val doc = data.ownerDoc(owner, n, data.entry(n)?.doc.orEmpty())
            var summary = data.summary(n)
            if (kind == Kind.Opt && (summary.isEmpty() || summary == n)) summary = optSummary(doc)
            Suggestion(n, kind, summary, data.usage(n), doc)
        }
    }

    private fun names(context: List<String>, synth: String): Pair<Kind, List<String>> {
        val none = Kind.Fn to emptyList<String>()
        if (context.isEmpty()) return none
        val partial = context.last()
        val words = wordsBeforePartial(context)
        val last = words.lastOrNull().orEmpty()
        val first = words.firstOrNull().orEmpty()
        val second = words.getOrElse(1) { "" }
        val docOpts = resolveFnOpts(context, data.fnOpts)
        var ctx = when (resolveArgKind(context, data.argKinds)) {
            "Sample" -> "Sample"; "CuePath" -> "CuePath"; "Fx" -> "FX"; "Synth" -> "Synth"; "Scale" -> "Scale"; "Chord" -> "Chord"
            "LinkAudioPeer", "LinkAudioChannel", "Track" -> return none
            else -> "Func"
        }
        if (ctx == "Func") {
            when {
                last == "sync:" -> ctx = "CuePath"
                (first == "midi" || first.startsWith("midi_") || first == "use_midi_defaults" || first == "with_midi_defaults") && last == "port:" -> ctx = "MidiOuts"
                last == "load_example" -> ctx = "Examples"
                last == "use_random_source" || last == "with_random_source" -> ctx = "RandomSource"
                last == "use_tuning" || last == "with_tuning" -> ctx = "Tuning"
                words.size >= 2 && first == "with_fx" -> {
                    if (last.endsWith(":")) return none
                    data.fxArgs[second]?.let { return Kind.Opt to it }
                }
                words.size >= 2 && first == "synth" -> {
                    if (last.endsWith(":")) return none
                    data.synthArgs[second]?.let { return Kind.Opt to it }
                }
                words.size >= 2 && (first == "play" || first == "control") -> {
                    if (last.endsWith(":")) return none
                    if (synth.isNotEmpty()) data.synthArgs[":$synth"]?.let { return Kind.Opt to it }
                    ctx = "PlayParam"
                }
                words.size >= 2 && first == "sample" -> {
                    if (last.endsWith(":")) return none
                    ctx = "SampleParam"
                }
                first == "use_sample_defaults" || first == "with_sample_defaults" -> {
                    if (last.endsWith(":")) return none
                    ctx = "SampleParam"
                }
                words.size >= 2 && first == "midi" -> {
                    if (last.endsWith(":")) return none
                    ctx = "MidiParam"
                }
                docOpts.isNotEmpty() -> return Kind.Opt to docOpts
                context.size > 1 -> if (atOptKeySlot(context) || partial.length <= 2) return none
            }
        }
        val kind = when (ctx) {
            "FX" -> Kind.Fx; "Synth" -> Kind.Synth; "Sample" -> Kind.Sample; "Chord" -> Kind.Chord; "Scale" -> Kind.Scale
            "Tuning" -> Kind.Tuning; "Examples" -> Kind.Example; "MidiOuts" -> Kind.Port; "CuePath" -> Kind.Cue
            "PlayParam", "SampleParam", "MidiParam", "RandomSource" -> Kind.Opt
            else -> Kind.Fn
        }
        return kind to keywords.getValue(ctx)
    }

    /**
     * What to offer at [caret] in [code], as Sonic Pi's editor offers it
     * unasked (cm.js's source): nothing in a comment, after a closed value or
     * right after a comma; a function's name once two letters of it are typed;
     * a slot's values (a sample, a synth, an opt, a note) as soon as it opens.
     * Ranked by native's fuzzy tiers. Null when there is nothing to offer.
     */
    fun suggest(code: String, caret: Int): Suggestions? {
        if (caret < 0 || caret > code.length) return null
        val lineStart = code.lastIndexOf('\n', caret - 1) + 1
        val lineEnd = code.indexOf('\n', caret).let { if (it < 0) code.length else it }
        val line = code.substring(lineStart, lineEnd)
        val col = caret - lineStart
        val scan = scanLineToCaret(line, col)
        if (scan.inComment || (!scan.inString && caretAfterClosedValue(line, col))) return null
        if (line.getOrNull(col - 1) == ',') return null
        val context = lineToContext(line, col)
        val items = completionsFor(context, synthAt(code, caret))
        if (items.isEmpty()) return null
        val kind = items[0].kind
        if (scan.inString && kind != Kind.Cue && kind != Kind.Port && kind != Kind.Sample) return null
        var partial = context.lastOrNull().orEmpty()
        val end = tokenEndAtCaret(line, col)
        var start = end - partial.length
        if (start < 0 || start > col) start = col
        var pool = items
        if (scan.inString && (partial.startsWith('"') || partial.startsWith('\''))) {
            val strings = items.filter { it.text.length > 1 && it.text[0] == partial[0] }
            if (kind == Kind.Cue || kind == Kind.Port) {
                if (strings.isEmpty()) return null
                pool = strings
            } else {
                start += 1
                partial = partial.substring(1)
            }
        }
        val typed = line.substring(start, col)
        if (kind == Kind.Fn && typed.length < 2) return null
        val seenNotes = HashSet<String>()
        val scored = ArrayList<Pair<Int, Suggestion>>()
        for (it in pool) {
            val score = if (typed.isEmpty()) 0 else fuzzyMatch(typed, it.text) ?: continue
            if (it.kind == Kind.Note && !seenNotes.add(if (it.text[0].isDigit()) it.text else it.summary)) continue
            scored += score to it
        }
        if (scored.isEmpty()) return null
        // the one word offered is the one already written: taking it would change nothing
        if (scored.size == 1 && scored[0].second.text == line.substring(start, end)) return null
        if (typed.isNotEmpty()) scored.sortByDescending { it.first }
        return Suggestions(lineStart + start, lineStart + end, scored.take(400).map { it.second })
    }

    /**
     * What the word under [caret] is, for the doc strip under the editor: a
     * function, a synth, FX or sample (`:name`), or an opt (`name:`), its doc
     * the one for the synth or FX it is given to. Null for anything else.
     */
    fun infoAt(code: String, caret: Int): WordInfo? {
        val lineStart = code.lastIndexOf('\n', caret - 1) + 1
        val lineEnd = code.indexOf('\n', caret).let { if (it < 0) code.length else it }
        val line = code.substring(lineStart, lineEnd)
        val word = wordAt(line, caret - lineStart) ?: return null
        val context = lineToContext(line, word.end)
        return when {
            word.text.endsWith(":") -> {
                val owner = ownerForContext(context, synthAt(code, caret))
                val doc = data.ownerDoc(owner, word.text, data.entry(word.text)?.doc.orEmpty())
                if (doc.isEmpty()) null else WordInfo(word.text, Kind.Opt, optSummary(doc), "", doc)
            }
            else -> {
                val e = data.entry(word.text) ?: return null
                val kind = when (word.text) {
                    in data.synthArgs -> Kind.Synth
                    in data.fxArgs -> Kind.Fx
                    else -> if (word.text.startsWith(":")) Kind.Sample else Kind.Fn
                }
                WordInfo(word.text, kind, e.summary, e.usage, e.doc)
            }
        }
    }
}

private fun enumLabelFor(doc: String, value: String): String {
    if (doc.isEmpty() || value.isEmpty()) return ""
    val m = Regex("\\b${Regex.escape(value)}\\b\\s*=?\\s*([A-Za-z][A-Za-z ]*?)(?=[,.;)<]|\\s+\\d|\\s+and\\b|$)").find(doc)
    return m?.groupValues?.get(1)?.trim().orEmpty()
}

internal data class Word(val text: String, val start: Int, val end: Int)

/** The name under the caret: `:symbol`, an opt `key:`, or a word (with a trailing `?` or `!`). Null in strings and comments. */
internal fun wordAt(line: String, col: Int): Word? {
    fun w(i: Int) = i in line.indices && (line[i].isLetterOrDigit() || line[i] == '_')
    var s = col.coerceIn(0, line.length)
    if (!w(s) && s > 0 && w(s - 1)) s--
    else if (!w(s) && line.getOrNull(s) == ':' && w(s + 1)) s++
    if (!w(s)) return null
    var e = s
    while (s > 0 && w(s - 1)) s--
    while (e < line.length && w(e)) e++
    if (line[s].isDigit()) return null
    val scan = scanLineToCaret(line, s)
    if (scan.inComment || scan.inString) return null
    val name = line.substring(s, e)
    if (line.getOrNull(s - 1) == ':' && line.getOrNull(s - 2) != ':') return Word(":$name", s - 1, e)
    if (line.getOrNull(e) == ':' && line.getOrNull(e + 1) != ':') return Word("$name:", s, e + 1)
    if (line.getOrNull(e) == '?' || line.getOrNull(e) == '!') return Word(name + line[e], s, e + 1)
    return Word(name, s, e)
}

/**
 * [value] with [item] taken in place of the word it completes, as cm.js
 * applies one: an opt goes in with a space after it, ready for its value,
 * and a string written over one already open takes its closing quote.
 */
fun accept(value: TextFieldValue, at: Suggestions, item: Suggestion): TextFieldValue {
    val text = value.text
    val chain = item.kind == Kind.Opt || (item.kind == Kind.Fn && item.text.endsWith(":"))
    val insert = if (chain) "${item.text} " else item.text
    var to = at.to.coerceAtMost(text.length)
    val quote = text.getOrNull(at.from)
    if ((quote == '"' || quote == '\'') && insert.length > 1 && insert.first() == quote && insert.last() == quote && text.getOrNull(to) == quote) to++
    return value.copy(
        text = text.substring(0, at.from) + insert + text.substring(to),
        selection = TextRange(at.from + insert.length),
    )
}
