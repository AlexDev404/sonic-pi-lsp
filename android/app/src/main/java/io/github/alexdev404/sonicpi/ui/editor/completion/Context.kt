// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.editor.completion

// Where the caret is, in words: a port of Sonic Pi's
// app/web/app/src/completion/context.js, itself a function-for-function port
// of the desktop GUI's app/gui/utils/completion_context.cpp, so this editor
// reads a line the way Sonic Pi's own editors do.

/** What the text before the caret is inside: a string, a comment, brackets. */
data class LineScan(val inComment: Boolean, val inString: Boolean, val bracketDepth: Int)

fun scanLineToCaret(line: String, caretCol: Int): LineScan {
    var inComment = false
    var inString = false
    var quote = ' '
    var depth = 0
    val end = caretCol.coerceIn(0, line.length)
    var i = 0
    while (i < end) {
        val c = line[i]
        if (inString) {
            if (c == '\\') { i += 2; continue }
            if (c == quote) inString = false
            i++; continue
        }
        if (c == '#') { inComment = true; break }
        if (c == '"' || c == '\'') { inString = true; quote = c; i++; continue }
        if (c == '(' || c == '[' || c == '{') depth++
        else if ((c == ')' || c == ']' || c == '}') && depth > 0) depth--
        i++
    }
    return LineScan(inComment, inString, depth)
}

private fun isTokenSeparator(c: Char) = c in " \t\n\r,(){}[]\"'#"
private fun isWordChar(c: Char) = c.isLetterOrDigit() || c == '_'

/** The end of the token the caret is in: `lpf: 7|0` completes 70, not 7. */
fun tokenEndAtCaret(line: String, caretCol: Int): Int {
    var end = caretCol.coerceIn(0, line.length)
    while (end < line.length && !isTokenSeparator(line[end])) end++
    return end
}

private val MODIFIERS = setOf("if", "unless", "while", "until", "and", "or", "then", "do")

/**
 * The call the caret is in, as tokens: the function and its arguments so far,
 * the last token being the partial under the caret.
 * `play 60, amp: 0.5, cut|` gives `[play, 60, amp:, 0.5, cut]`.
 */
fun lineToContext(fullLine: String, caretCol: Int): List<String> {
    var line = fullLine.substring(0, tokenEndAtCaret(fullLine, caretCol))
    // A statement modifier or operator ends the call's arguments: what follows is a fresh expression.
    run {
        var depth = 0
        var cut = -1
        var quote: Char? = null
        var i = 0
        while (i < line.length) {
            val c = line[i]
            if (quote != null) { if (c == quote) quote = null; i++; continue }
            if (c == '"' || c == '\'') { quote = c; i++; continue }
            if (c == '(' || c == '[' || c == '{') { depth++; i++; continue }
            if (c == ')' || c == ']' || c == '}') { if (depth > 0) depth--; i++; continue }
            if (depth != 0) { i++; continue }
            if (c == ';') { cut = i + 1; i++; continue }
            if ((c == '&' && line.getOrNull(i + 1) == '&') || (c == '|' && line.getOrNull(i + 1) == '|')) { cut = i + 2; i += 2; continue }
            val startsWord = (c.isLetter() || c == '_') && (i == 0 || !isWordChar(line[i - 1]))
            if (!startsWord) { i++; continue }
            var j = i
            while (j < line.length && isWordChar(line[j])) j++
            if (line.substring(i, j) in MODIFIERS) cut = j
            i = j
        }
        if (cut >= 0) {
            while (cut < line.length && line[cut] == ' ') cut++
            line = line.substring(cut)
        }
    }
    // Nested calls resolve to the innermost: `play (scale ` completes scale's args.
    val open = ArrayDeque<Int>()
    for ((i, c) in line.withIndex()) {
        if (c == '(' || c == '[' || c == '{') open.addLast(i)
        else if ((c == ')' || c == ']' || c == '}') && open.isNotEmpty()) open.removeLast()
    }
    if (open.isNotEmpty()) {
        val innermost = open.last()
        var s = innermost
        while (s > 0 && isWordChar(line[s - 1])) s--
        val fn = line.substring(s, innermost)
        line = line.substring(innermost + 1)
        if (fn.isNotEmpty()) line = "$fn $line"
    }
    // Split on spaces, commas and brackets, a string staying one token; a run of separators is one split.
    val out = mutableListOf<String>()
    val cur = StringBuilder()
    var quote: Char? = null
    var inSeparators = false
    var i = 0
    while (i < line.length) {
        val c = line[i]
        if (quote != null) {
            cur.append(c)
            if (c == '\\' && i + 1 < line.length) { cur.append(line[i + 1]); i += 2; continue }
            if (c == quote) quote = null
            i++; continue
        }
        if (c in " ,(){}") {
            if (!inSeparators) { out += cur.toString(); cur.clear(); inSeparators = true }
            i++; continue
        }
        inSeparators = false
        if (c == '"' || c == '\'') quote = c
        cur.append(c)
        i++
    }
    out += cur.toString()
    return out
}

internal fun wordsBeforePartial(context: List<String>) = context.dropLast(1).filter { it.isNotEmpty() }

internal fun isOptKey(w: String) = w.length > 1 && w.endsWith(":") && !w.startsWith(":")

/** What kind of value the caret's argument slot takes, from the arg-kinds table. */
fun resolveArgKind(context: List<String>, table: Map<String, List<String>>): String {
    if (context.isEmpty()) return "None"
    val words = wordsBeforePartial(context)
    var argIndex = 0
    for (i in words.indices.reversed()) {
        if (words[i].endsWith(":")) return "None"
        val kinds = table[words[i]]
        if (kinds != null) return kinds.getOrElse(argIndex) { "None" }
        argIndex++
    }
    return "None"
}

/** The documented opts of the function the caret is in, once past its first argument. */
fun resolveFnOpts(context: List<String>, table: Map<String, List<String>>): List<String> {
    val words = wordsBeforePartial(context)
    if (words.isEmpty() || isOptKey(words.last())) return emptyList()
    for (i in words.indices.reversed()) {
        val opts = table[words[i]]
        if (opts != null) return if (i < words.size - 1) opts else emptyList()
    }
    return emptyList()
}

/** True where another opt key goes: an opt has been given and its value too. */
fun atOptKeySlot(context: List<String>): Boolean {
    var seen = false
    var last = ""
    for (w in context.dropLast(1)) {
        if (w.isEmpty()) continue
        if (isOptKey(w)) seen = true
        last = w
    }
    return seen && !isOptKey(last)
}

/** The caret sits right after a closing bracket or quote: no completion. */
fun caretAfterClosedValue(line: String, caretCol: Int): Boolean {
    val i = caretCol.coerceIn(0, line.length)
    return i > 0 && line[i - 1] in ")]}\"'"
}

/**
 * Fuzzy subsequence match and rank: every char of [pat] in [text], in order,
 * case-insensitive. Tiers: exact > prefix > substring (word boundary > mid
 * word) > scattered, shorter winning ties. The score, or null for no match.
 */
fun fuzzyMatch(pat: String, text: String): Int? {
    if (pat.isEmpty()) return 0
    fun isBoundary(i: Int) = i == 0 || text[i - 1] in ":_ -/.!?"
    var ti = 0
    var pi = 0
    var run = 0
    var base = 0
    var gaps = 0
    while (ti < text.length && pi < pat.length) {
        if (text[ti].lowercaseChar() == pat[pi].lowercaseChar()) {
            if (isBoundary(ti) && ti > 0) base += 8
            run++
            base += 1 + run * 3
            pi++
        } else {
            if (pi > 0) gaps++
            run = 0
        }
        ti++
    }
    if (pi != pat.length) return null
    base -= gaps * 2
    val t = text.lowercase()
    val p = pat.lowercase()
    val tier = when {
        t == p -> 1000
        t.startsWith(p) -> 600
        else -> t.indexOf(p).let { if (it < 0) 0 else if (isBoundary(it)) 400 else 250 }
    }
    return tier + base - text.length
}
