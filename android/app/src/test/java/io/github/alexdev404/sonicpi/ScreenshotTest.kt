// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import androidx.test.core.app.ApplicationProvider
import com.github.takahirom.roborazzi.captureRoboImage
import io.github.alexdev404.sonicpi.data.BufferStore
import io.github.alexdev404.sonicpi.data.Library
import io.github.alexdev404.sonicpi.engine.EngineStatus
import io.github.alexdev404.sonicpi.engine.LogEntry
import io.github.alexdev404.sonicpi.engine.LogKind
import io.github.alexdev404.sonicpi.ui.AppActions
import io.github.alexdev404.sonicpi.ui.AppUiState
import io.github.alexdev404.sonicpi.ui.Destination
import io.github.alexdev404.sonicpi.ui.SonicPiApp
import io.github.alexdev404.sonicpi.ui.theme.SonicPiTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * The app's screens, drawn on the JVM at a phone's and a tablet's size, with
 * a program running and its log. Written to build/outputs/roborazzi.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [36])
class ScreenshotTest {
    @get:Rule val compose = createComposeRule()

    private val library by lazy { Library(ApplicationProvider.getApplicationContext<android.app.Application>().assets) }

    private fun state(): AppUiState {
        val log = listOf(
            LogEntry(0, LogKind.Output, 1, 0.0, 3, "", "\"hello from Sonic Pi\""),
            LogEntry(1, LogKind.Synth, 1, 0.0, 6, "drums", "sample :bd_haus, {amp: 1.5}"),
            LogEntry(2, LogKind.Synth, 1, 0.0, 11, "melody", "synth :prophet, {note: 52.0, release: 0.3, cutoff: 94.0}"),
            LogEntry(3, LogKind.Cue, 1, 0.0, 0, "drums", "/live_loop/drums"),
            LogEntry(4, LogKind.Synth, 1, 0.25, 11, "melody", "synth :prophet, {note: 57.0, release: 0.3, cutoff: 81.0}"),
            LogEntry(5, LogKind.Synth, 1, 0.5, 11, "melody", "synth :prophet, {note: 55.0, release: 0.3, cutoff: 102.0}"),
            LogEntry(6, LogKind.Synth, 1, 0.6, 6, "drums", "sample :bd_haus, {amp: 1.5}"),
        )
        val buffers = List(BufferStore.COUNT) { i ->
            if (i == 0) TextFieldValue(BufferStore.WELCOME, TextRange(0)) else if (i == 3) TextFieldValue("play 60") else TextFieldValue("")
        }
        return AppUiState(
            status = EngineStatus.Ready, running = true, log = log, buffers = buffers, current = 0, errorLine = 0,
            fontSize = 14, functions = library.functionNames, sections = library.sections, canUndo = true, canRedo = false,
        )
    }

    private fun shoot(name: String, destination: Destination, state: AppUiState = state()) {
        compose.setContent { SonicPiTheme { SonicPiApp(state, AppActions(), initial = destination) } }
        compose.onRoot().captureRoboImage("build/outputs/roborazzi/$name.png")
    }

    @Config(qualifiers = "w411dp-h891dp-xxhdpi")
    @Test fun phoneCode() = shoot("phone-code", Destination.Code)

    @Config(qualifiers = "w411dp-h891dp-xxhdpi")
    @Test fun phoneLog() = shoot("phone-log", Destination.Log)

    @Config(qualifiers = "w411dp-h891dp-xxhdpi")
    @Test fun phoneLearn() = shoot("phone-learn", Destination.Learn)

    @Config(qualifiers = "w411dp-h891dp-xxhdpi")
    @Test fun phoneError() {
        val s = state()
        shoot("phone-error", Destination.Code, s.copy(
            running = false, errorLine = 11,
            log = s.log + LogEntry(9, LogKind.Error, 2, 1.0, 11, "", "NoMethodError: undefined method 'choos'"),
        ))
    }

    @Config(qualifiers = "w411dp-h891dp-xxhdpi")
    @Test fun phoneBooting() = shoot("phone-booting", Destination.Code, state().copy(status = EngineStatus.Preparing(0.42f, "Unpacking sounds")))

    @Config(qualifiers = "w1280dp-h800dp-xhdpi")
    @Test fun tabletCode() = shoot("tablet-code", Destination.Code)
}
