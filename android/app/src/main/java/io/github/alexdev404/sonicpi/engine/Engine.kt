// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.engine

import kotlinx.coroutines.flow.StateFlow

/** What a line of the log is (sonic_pi_core.h, Event::Kind, in the same order). */
enum class LogKind { Output, Log, Error, Cue, Synth, State, Engine }

data class LogEntry(
    val id: Long,
    val kind: LogKind,
    val job: Int,
    /** Seconds since the session's first Run. */
    val time: Double,
    /** The program's line, 0 for none. */
    val line: Int,
    /** The thread's name (a live_loop's), or "". */
    val thread: String,
    val text: String,
)

sealed interface EngineStatus {
    /** Unpacking the sounds on first launch, then booting. */
    data class Preparing(val progress: Float, val message: String) : EngineStatus
    data object Ready : EngineStatus
    data class Failed(val message: String) : EngineStatus
}

/** Sonic Pi, as the UI sees it. NativeEngine is the real one; tests use a fake. */
interface SonicPiEngine {
    val status: StateFlow<EngineStatus>
    val running: StateFlow<Boolean>
    val log: StateFlow<List<LogEntry>>
    fun run(code: String)
    fun stop()
    fun clearLog()
}
