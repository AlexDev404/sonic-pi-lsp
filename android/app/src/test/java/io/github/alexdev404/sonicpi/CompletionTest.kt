// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi

import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import androidx.test.core.app.ApplicationProvider
import io.github.alexdev404.sonicpi.data.Library
import io.github.alexdev404.sonicpi.ui.editor.completion.Kind
import io.github.alexdev404.sonicpi.ui.editor.completion.accept
import io.github.alexdev404.sonicpi.ui.editor.completion.fuzzyMatch
import io.github.alexdev404.sonicpi.ui.editor.completion.lineToContext
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** The editor's completion, on Sonic Pi's own data (completion.json and the reference, from the assets). */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [36])
class CompletionTest {
    private val engine by lazy { Library(ApplicationProvider.getApplicationContext<android.app.Application>().assets).completion }

    /** The suggestions with the caret at the end of [code]. */
    private fun at(code: String) = engine.suggest(code, code.length)
    private fun names(code: String) = at(code)?.items?.map { it.text }.orEmpty()

    @Test fun readsALineAsTheDesktopDoes() {
        assertEquals(listOf("play", "60", "amp:", "0.5", "cut"), lineToContext("play 60, amp: 0.5, cut", 22))
        assertEquals(listOf("scale", ":c4", ""), lineToContext("play (scale :c4, ", 17))
        assertEquals(listOf("sample", ":bd"), lineToContext("sleep 1 if sample :bd", 21))
    }

    @Test fun ranksAsTheDesktopDoes() {
        assertTrue(fuzzyMatch("sam", "sample")!! > fuzzyMatch("sam", "use_sample_bpm")!!)
        assertTrue(fuzzyMatch("sam", "use_sample_bpm")!! > fuzzyMatch("sam", "bsam")!!)
        assertNull(fuzzyMatch("xyz", "play"))
    }

    @Test fun samplesAfterSample() {
        val s = at("sample :bd_ha")!!
        assertEquals(":bd_haus", s.items.first().text)
        assertEquals(Kind.Sample, s.items.first().kind)
        assertEquals(7, s.from)
        assertEquals(13, s.to)
    }

    @Test fun synthsAfterUseSynth() {
        assertEquals(":prophet", names("use_synth :proph").first())
    }

    @Test fun functionsOnceTwoLettersAreTyped() {
        assertNull(at("p"))
        assertEquals("play", names("pla").first())
        assertEquals("live_loop", names("live_l").first())
    }

    @Test fun theSynthsOwnOptsAfterPlay() {
        val opts = names("use_synth :prophet\nplay 60, ")
        assertTrue("cutoff: in $opts", "cutoff:" in opts)
        assertTrue("res: in $opts", "res:" in opts)
        assertEquals(Kind.Opt, at("use_synth :prophet\nplay 60, ")!!.items.first().kind)
    }

    @Test fun anFxsOpts() {
        assertEquals("room:", names("with_fx :reverb, roo").first())
    }

    @Test fun notesAfterPlay() {
        val s = at("play ")!!
        assertEquals(Kind.Note, s.items.first().kind)
        assertTrue("60" in s.items.map { it.text })
    }

    @Test fun nothingInACommentOrRightAfterAComma() {
        assertNull(at("# sample :bd"))
        assertNull(at("play 60,"))
    }

    @Test fun anOptGoesInReadyForItsValue() {
        val code = "use_synth :prophet\nplay 60, cut"
        val s = at(code)!!
        val cutoff = s.items.first { it.text == "cutoff:" }
        assertEquals("use_synth :prophet\nplay 60, cutoff: ", accept(TextFieldValue(code, TextRange(code.length)), s, cutoff).text)
    }

    @Test fun whatTheWordUnderTheCaretIs() {
        val play = engine.infoAt("play 60", 2)
        assertNotNull(play)
        assertEquals("Play current synth", play!!.summary)
        assertEquals(Kind.Synth, engine.infoAt("use_synth :prophet", 13)!!.kind)
        val cutoff = engine.infoAt("use_synth :prophet\nplay 60, cutoff: 80", 30)!!
        assertEquals("cutoff:", cutoff.title)
        assertTrue(cutoff.doc.isNotEmpty())
        assertNull(engine.infoAt("# play", 3))
    }
}
