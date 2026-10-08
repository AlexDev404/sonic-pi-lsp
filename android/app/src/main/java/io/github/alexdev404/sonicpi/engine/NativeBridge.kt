// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.engine

/** libsonicpi.so (src/main/cpp/sonicpi_jni.cpp): one engine per process. */
object NativeBridge {
    init {
        System.loadLibrary("sonicpi")
    }

    /** Boots the engine and the language runtime over the extracted assets: "" when ready, else why not. */
    external fun nativeBoot(assets: String, sampleRate: Int): String

    /** Opens the audio output: "" when playing, else why not. */
    external fun nativeStartAudio(): String
    external fun nativeStopAudio()

    external fun nativeRun(code: String)
    external fun nativeStop()
    external fun nativeIsRunning(): Boolean

    /** Events since the last poll: kind, job, time, line, thread, text, separated by U+001F. */
    external fun nativePollEvents(): Array<String>
}
