// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.engine

import android.content.Context
import android.media.AudioManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** The engine in libsonicpi.so: booted off the main thread, its events polled into the log. */
class NativeEngine(private val context: Context, private val scope: CoroutineScope) : SonicPiEngine {
    private val _status = MutableStateFlow<EngineStatus>(EngineStatus.Preparing(0f, "Starting"))
    private val _running = MutableStateFlow(false)
    private val _log = MutableStateFlow<List<LogEntry>>(emptyList())
    override val status: StateFlow<EngineStatus> = _status
    override val running: StateFlow<Boolean> = _running
    override val log: StateFlow<List<LogEntry>> = _log
    private var nextId = 0L
    private var audioOn = false

    init {
        scope.launch { boot() }
    }

    private suspend fun boot() = withContext(Dispatchers.IO) {
        try {
            val installer = AssetInstaller(context.assets, context.filesDir)
            val root = installer.install { _status.value = EngineStatus.Preparing(it, "Unpacking sounds") }
            _status.value = EngineStatus.Preparing(1f, "Starting the engine")
            val err = NativeBridge.nativeBoot(root.path, deviceSampleRate())
            if (err.isNotEmpty()) {
                _status.value = EngineStatus.Failed(err)
                return@withContext
            }
            val audio = NativeBridge.nativeStartAudio()
            if (audio.isNotEmpty()) {
                _status.value = EngineStatus.Failed("No audio output: $audio")
                return@withContext
            }
            audioOn = true
            _status.value = EngineStatus.Ready
        } catch (t: Throwable) {
            _status.value = EngineStatus.Failed(t.message ?: t.toString())
            return@withContext
        }
        scope.launch { poll() }
    }

    // The rate the device runs at, so Android need not resample.
    private fun deviceSampleRate(): Int {
        val am = context.getSystemService(AudioManager::class.java)
        return am?.getProperty(AudioManager.PROPERTY_OUTPUT_SAMPLE_RATE)?.toIntOrNull() ?: 48000
    }

    private suspend fun poll() {
        while (scope.isActive) {
            val rows = withContext(Dispatchers.IO) { NativeBridge.nativePollEvents() }
            if (rows.isNotEmpty()) {
                val added = rows.mapNotNull { parse(it) }
                _log.value = (_log.value + added).takeLast(MAX_LOG)
            }
            _running.value = NativeBridge.nativeIsRunning()
            delay(if (_running.value) 40 else 120)
        }
    }

    private fun parse(row: String): LogEntry? {
        val f = row.split('\u001f', limit = 6)
        if (f.size < 6) return null
        val kind = LogKind.entries.getOrNull(f[0].toIntOrNull() ?: return null) ?: return null
        return LogEntry(nextId++, kind, f[1].toIntOrNull() ?: 0, f[2].toDoubleOrNull() ?: 0.0, f[3].toIntOrNull() ?: 0, f[4], f[5])
    }

    override fun run(code: String) {
        if (_status.value != EngineStatus.Ready) return
        if (!audioOn) resumeAudio()
        NativeBridge.nativeRun(code)
    }

    override fun stop() {
        if (_status.value == EngineStatus.Ready) NativeBridge.nativeStop()
    }

    override fun clearLog() {
        _log.value = emptyList()
    }

    /** The app went to the background with nothing playing: the audio stream closes, saving power. */
    fun pauseAudioIfIdle() {
        if (_status.value != EngineStatus.Ready || !audioOn || NativeBridge.nativeIsRunning()) return
        NativeBridge.nativeStopAudio()
        audioOn = false
    }

    fun resumeAudio() {
        if (_status.value != EngineStatus.Ready || audioOn) return
        audioOn = NativeBridge.nativeStartAudio().isEmpty()
    }

    private companion object {
        const val MAX_LOG = 1500
    }
}
