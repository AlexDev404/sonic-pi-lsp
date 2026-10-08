// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi

import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import io.github.alexdev404.sonicpi.ui.editor.Highlighter
import io.github.alexdev404.sonicpi.ui.editor.TokenKind
import io.github.alexdev404.sonicpi.ui.editor.autoIndent
import io.github.alexdev404.sonicpi.ui.editor.indentAfter
import io.github.alexdev404.sonicpi.ui.editor.insert
import org.junit.Assert.assertEquals
import org.junit.Test

class EditorTest {
    private fun kinds(code: String, fns: Set<String> = setOf("play", "sample", "live_loop", "sleep")) =
        Highlighter.tokens(code, fns).map { code.substring(it.start, it.end) to it.kind }

    @Test fun coloursCodeAsSonicPiDoes() {
        assertEquals(
            listOf(
                "live_loop" to TokenKind.Function, ":drums" to TokenKind.Symbol, "do" to TokenKind.Keyword,
                "sample" to TokenKind.Function, ":bd_haus" to TokenKind.Symbol, "amp:" to TokenKind.OptKey, "1.5" to TokenKind.Number,
                "# kick" to TokenKind.Comment, "end" to TokenKind.Keyword,
            ),
            kinds("live_loop :drums do\n  sample :bd_haus, amp: 1.5 # kick\nend"),
        )
    }

    @Test fun stringsAndCommentsHideWhatIsInThem() {
        assertEquals(listOf("puts" to TokenKind.Plain, "\"play :x # no\"" to TokenKind.Str).filter { it.second != TokenKind.Plain },
            kinds("puts \"play :x # no\""))
        assertEquals(listOf("# play 60" to TokenKind.Comment), kinds("# play 60"))
    }

    @Test fun indentsAfterDo() {
        assertEquals("  ", indentAfter("live_loop :a do"))
        assertEquals("    ", indentAfter("  3.times do |i|"))
        assertEquals("  ", indentAfter("  play 60"))
        assertEquals("", indentAfter("end"))
    }

    @Test fun aNewlineTakesTheIndent() {
        val old = TextFieldValue("live_loop :a do", TextRange(15))
        val typed = TextFieldValue("live_loop :a do\n", TextRange(16))
        val out = autoIndent(old, typed)
        assertEquals("live_loop :a do\n  ", out.text)
        assertEquals(TextRange(18), out.selection)
    }

    @Test fun insertReplacesTheSelection() {
        assertEquals("play :c4", TextFieldValue("play 60", TextRange(5, 7)).insert(":c4").text)
    }
}
