// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.runtime.getValue
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.alexdev404.sonicpi.ui.AppActions
import io.github.alexdev404.sonicpi.ui.AppUiState
import io.github.alexdev404.sonicpi.ui.SonicPiApp
import io.github.alexdev404.sonicpi.ui.theme.SonicPiTheme

class MainActivity : ComponentActivity() {
    private val vm: AppViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        // Edge to edge, as Android 15+ requires of an app targeting it: the
        // screens pad themselves for the system bars and the keyboard.
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.dark(android.graphics.Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.dark(android.graphics.Color.TRANSPARENT),
        )
        super.onCreate(savedInstanceState)

        // In the background with nothing playing, the audio stream closes; it reopens on return.
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStart(owner: LifecycleOwner) = vm.engine.resumeAudio()
            override fun onStop(owner: LifecycleOwner) = vm.engine.pauseAudioIfIdle()
        })

        setContent {
            SonicPiTheme {
                val status by vm.engine.status.collectAsStateWithLifecycle()
                val running by vm.engine.running.collectAsStateWithLifecycle()
                val log by vm.engine.log.collectAsStateWithLifecycle()
                // Files go in and out through the system's document picker: the
                // user grants this one file, and the app asks for no storage permission.
                val open = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri -> uri?.let(vm::importFrom) }
                val save = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("text/x-ruby")) { uri -> uri?.let(vm::exportTo) }
                SonicPiApp(
                    state = AppUiState(
                        status = status, running = running, log = log,
                        buffers = vm.buffers, current = vm.current, errorLine = vm.errorLine, fontSize = vm.fontSize,
                        functions = vm.functions, sections = vm.sections, canUndo = vm.canUndo, canRedo = vm.canRedo,
                        completion = vm.completion,
                    ),
                    actions = AppActions(
                        run = vm::run, stop = vm::stop, selectBuffer = vm::selectBuffer, edit = vm::edit, insert = vm::insert,
                        undo = vm::undo, redo = vm::redo, openCode = vm::openCode, play = vm::play, clearLog = vm::clearLog,
                        fontSize = vm::changeFontSize,
                        openFile = { open.launch(arrayOf("text/*", "application/x-ruby", "application/octet-stream")) },
                        saveFile = { save.launch("buffer_${vm.current}.rb") },
                    ),
                )
            }
        }
    }
}
