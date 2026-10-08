// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.data

import java.io.File

/**
 * Ten buffers, as Sonic Pi has, kept as files in the app's private storage
 * (files/buffers/buffer_N.rb). Private storage needs no permission; opening
 * and saving other files goes through the system's document picker.
 */
class BufferStore(filesDir: File) {
    private val dir = File(filesDir, "buffers").apply { mkdirs() }

    fun load(index: Int): String {
        val f = file(index)
        return if (f.isFile) f.readText() else if (index == 0) WELCOME else ""
    }

    fun save(index: Int, text: String) {
        val f = file(index)
        val tmp = File(dir, "${f.name}.tmp")
        tmp.writeText(text)
        tmp.renameTo(f)
    }

    private fun file(index: Int) = File(dir, "buffer_$index.rb")

    companion object {
        const val COUNT = 10

        val WELCOME = """
            |# Welcome to Sonic Pi
            |# Press Run to hear this, Stop to stop.
            |
            |use_bpm 100
            |
            |live_loop :drums do
            |  sample :bd_haus, amp: 1.5
            |  sleep 1
            |end
            |
            |live_loop :melody do
            |  use_synth :prophet
            |  play scale(:e3, :minor_pentatonic).choose, release: 0.3, cutoff: rrand(70, 110)
            |  sleep 0.25
            |end
            |""".trimMargin()
    }
}
