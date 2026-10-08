// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.editor

import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.OffsetMapping
import androidx.compose.ui.text.input.TransformedText
import androidx.compose.ui.text.input.VisualTransformation
import io.github.alexdev404.sonicpi.ui.theme.SonicPiColors

/** What a stretch of code is, for its colour. */
enum class TokenKind { Comment, Str, Symbol, OptKey, Number, Keyword, Function, Plain }

data class Token(val start: Int, val end: Int, val kind: TokenKind)

/**
 * Sonic Pi's code, tokenised as its editor colours it: comments grey,
 * strings green, symbols and Sonic Pi's functions pink, numbers blue,
 * Ruby's keywords yellow.
 */
object Highlighter {
    val keywords = setOf(
        "do", "end", "if", "else", "elsif", "unless", "while", "until", "def", "return", "then", "case", "when",
        "in", "begin", "rescue", "ensure", "yield", "true", "false", "nil", "and", "or", "not", "self", "break",
        "next",
    )

    fun tokens(code: String, functions: Set<String>): List<Token> {
        val out = ArrayList<Token>()
        var i = 0
        val n = code.length
        fun isWord(c: Char) = c.isLetterOrDigit() || c == '_'
        while (i < n) {
            val c = code[i]
            when {
                c == '#' -> {
                    val end = code.indexOf('\n', i).let { if (it < 0) n else it }
                    out += Token(i, end, TokenKind.Comment)
                    i = end
                }
                c == '"' || c == '\'' -> {
                    var j = i + 1
                    while (j < n && code[j] != c && code[j] != '\n') j += if (code[j] == '\\') 2 else 1
                    val end = minOf(n, j + 1)
                    out += Token(i, end, TokenKind.Str)
                    i = end
                }
                c == ':' && i + 1 < n && (code[i + 1].isLetter() || code[i + 1] == '_') && (i == 0 || code[i - 1] != ':') -> {
                    var j = i + 1
                    while (j < n && isWord(code[j])) j++
                    if (j < n && (code[j] == '?' || code[j] == '!')) j++
                    out += Token(i, j, TokenKind.Symbol)
                    i = j
                }
                c.isDigit() && (i == 0 || !isWord(code[i - 1])) -> {
                    var j = i
                    while (j < n && (code[j].isDigit() || code[j] == '_' || (code[j] == '.' && j + 1 < n && code[j + 1].isDigit()))) j++
                    out += Token(i, j, TokenKind.Number)
                    i = j
                }
                c.isLetter() || c == '_' -> {
                    var j = i
                    while (j < n && isWord(code[j])) j++
                    if (j < n && (code[j] == '?' || code[j] == '!')) j++
                    val word = code.substring(i, j)
                    val kind = when {
                        j < n && code[j] == ':' && (j + 1 >= n || code[j + 1] != ':') -> TokenKind.OptKey
                        word in keywords -> TokenKind.Keyword
                        word in functions -> TokenKind.Function
                        else -> TokenKind.Plain
                    }
                    if (kind == TokenKind.OptKey) {
                        out += Token(i, j + 1, kind)
                        i = j + 1
                    } else {
                        if (kind != TokenKind.Plain) out += Token(i, j, kind)
                        i = j
                    }
                }
                else -> i++
            }
        }
        return out
    }

    fun style(kind: TokenKind): SpanStyle = when (kind) {
        TokenKind.Comment -> SpanStyle(color = SonicPiColors.Grey)
        TokenKind.Str -> SpanStyle(color = SonicPiColors.Green)
        TokenKind.Symbol -> SpanStyle(color = SonicPiColors.Pink)
        TokenKind.OptKey -> SpanStyle(color = SonicPiColors.Text)
        TokenKind.Number -> SpanStyle(color = SonicPiColors.Blue)
        TokenKind.Keyword -> SpanStyle(color = SonicPiColors.Yellow)
        TokenKind.Function -> SpanStyle(color = SonicPiColors.Pink, fontWeight = FontWeight.Bold)
        TokenKind.Plain -> SpanStyle()
    }

    fun highlight(code: String, functions: Set<String>): AnnotatedString = buildAnnotatedString {
        append(code)
        for (t in tokens(code, functions)) addStyle(style(t.kind), t.start, t.end)
    }
}

/** The editor's colouring, as a transformation that changes no offsets. */
class HighlightTransformation(private val functions: Set<String>) : VisualTransformation {
    override fun filter(text: AnnotatedString): TransformedText =
        TransformedText(Highlighter.highlight(text.text, functions), OffsetMapping.Identity)

    override fun equals(other: Any?) = other is HighlightTransformation && other.functions == functions
    override fun hashCode() = functions.hashCode()
}

/** The indentation a new line takes after the one the cursor was on: the same, and a step more after `do`. */
fun indentAfter(line: String): String {
    val base = line.takeWhile { it == ' ' }
    val code = line.substringBefore('#').trimEnd()
    val opens = code.endsWith(" do") || code == "do" || Regex("""\bdo\s*\|[^|]*\|$""").containsMatchIn(code) ||
        code.endsWith("{") || code.endsWith("[") || code.endsWith("(") ||
        Regex("""^\s*(if|unless|while|until|def|define|case|begin|else|elsif)\b""").containsMatchIn(code)
    return if (opens) "$base  " else base
}
