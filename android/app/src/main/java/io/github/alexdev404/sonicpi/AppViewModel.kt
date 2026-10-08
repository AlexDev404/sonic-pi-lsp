// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi

import android.app.Application
import android.net.Uri
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import io.github.alexdev404.sonicpi.data.BufferStore
import io.github.alexdev404.sonicpi.data.Library
import io.github.alexdev404.sonicpi.data.LibrarySection
import io.github.alexdev404.sonicpi.engine.LogKind
import io.github.alexdev404.sonicpi.engine.NativeEngine
import io.github.alexdev404.sonicpi.ui.editor.insert
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class AppViewModel(app: Application) : AndroidViewModel(app) {
    val engine = NativeEngine(app, viewModelScope)
    private val store = BufferStore(app.filesDir)
    private val prefs = app.getSharedPreferences("sonicpi", 0)

    val buffers = mutableStateListOf<TextFieldValue>().apply {
        repeat(BufferStore.COUNT) { add(TextFieldValue(store.load(it))) }
    }
    var current by mutableIntStateOf(prefs.getInt("buffer", 0).coerceIn(0, BufferStore.COUNT - 1))
        private set
    var fontSize by mutableIntStateOf(prefs.getInt("fontSize", 14))
        private set
    var errorLine by mutableIntStateOf(0)
        private set
    var functions by mutableStateOf<Set<String>>(emptySet())
        private set
    var sections by mutableStateOf<List<LibrarySection>>(emptyList())
        private set

    // Undo and redo, per buffer: snapshots, a burst of typing as one step.
    private val undo = List(BufferStore.COUNT) { ArrayDeque<TextFieldValue>() }
    private val redo = List(BufferStore.COUNT) { ArrayDeque<TextFieldValue>() }
    private var lastEdit = 0L
    var canUndo by mutableStateOf(false)
        private set
    var canRedo by mutableStateOf(false)
        private set

    private val saves = arrayOfNulls<Job>(BufferStore.COUNT)
    private var runBuffer = 0
    private var logMark = 0L

    init {
        viewModelScope.launch {
            val library = withContext(Dispatchers.IO) { Library(app.assets).also { it.sections } }
            functions = library.functionNames
            sections = library.sections
        }
        // An error after a Run marks its line in the buffer that ran.
        viewModelScope.launch {
            engine.log.collect { log ->
                val err = log.lastOrNull { it.kind == LogKind.Error && it.id >= logMark && it.line > 0 }
                if (err != null && runBuffer == current) errorLine = err.line
            }
        }
    }

    fun run() {
        errorLine = 0
        runBuffer = current
        logMark = engine.log.value.lastOrNull()?.id?.plus(1) ?: 0
        engine.run(buffers[current].text)
    }

    fun play(code: String) = engine.run(code)
    fun stop() = engine.stop()
    fun clearLog() = engine.clearLog()

    fun selectBuffer(i: Int) {
        current = i
        errorLine = 0
        prefs.edit().putInt("buffer", i).apply()
        updateHistoryFlags()
    }

    fun edit(value: TextFieldValue) {
        val old = buffers[current]
        if (value.text != old.text) {
            val now = System.currentTimeMillis()
            if (now - lastEdit > 1000 || undo[current].isEmpty()) {
                undo[current].addLast(old)
                if (undo[current].size > 200) undo[current].removeFirst()
            }
            lastEdit = now
            redo[current].clear()
            errorLine = 0
            scheduleSave(current, value.text)
        }
        buffers[current] = value
        updateHistoryFlags()
    }

    fun insert(text: String) {
        lastEdit = 0
        edit(buffers[current].insert(text))
    }

    /** Code from Learn: into the current buffer if it is empty, else appended after what is there. */
    fun openCode(code: String) {
        val b = buffers[current]
        val text = if (b.text.isBlank()) code.trimEnd() + "\n" else b.text.trimEnd() + "\n\n" + code.trimEnd() + "\n"
        lastEdit = 0
        edit(TextFieldValue(text, TextRange(text.length)))
    }

    fun undo() {
        val prev = undo[current].removeLastOrNull() ?: return
        redo[current].addLast(buffers[current])
        buffers[current] = prev
        scheduleSave(current, prev.text)
        updateHistoryFlags()
    }

    fun redo() {
        val next = redo[current].removeLastOrNull() ?: return
        undo[current].addLast(buffers[current])
        buffers[current] = next
        scheduleSave(current, next.text)
        updateHistoryFlags()
    }

    fun changeFontSize(delta: Int) {
        fontSize = (fontSize + delta).coerceIn(10, 28)
        prefs.edit().putInt("fontSize", fontSize).apply()
    }

    /** A file the user picked (the system's document picker: no storage permission) into the current buffer. */
    fun importFrom(uri: Uri) {
        viewModelScope.launch {
            val text = withContext(Dispatchers.IO) {
                getApplication<Application>().contentResolver.openInputStream(uri)?.bufferedReader()?.readText()
            } ?: return@launch
            lastEdit = 0
            edit(TextFieldValue(text))
        }
    }

    fun exportTo(uri: Uri) {
        val text = buffers[current].text
        viewModelScope.launch(Dispatchers.IO) {
            getApplication<Application>().contentResolver.openOutputStream(uri, "wt")?.use { it.write(text.toByteArray()) }
        }
    }

    private fun updateHistoryFlags() {
        canUndo = undo[current].isNotEmpty()
        canRedo = redo[current].isNotEmpty()
    }

    private fun scheduleSave(index: Int, text: String) {
        saves[index]?.cancel()
        saves[index] = viewModelScope.launch(Dispatchers.IO) {
            delay(400)
            store.save(index, text)
        }
    }
}
