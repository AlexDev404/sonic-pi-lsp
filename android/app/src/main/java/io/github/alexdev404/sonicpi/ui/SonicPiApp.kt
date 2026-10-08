// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui

import android.graphics.BitmapFactory
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.isImeVisible
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.alexdev404.sonicpi.data.LibrarySection
import io.github.alexdev404.sonicpi.engine.EngineStatus
import io.github.alexdev404.sonicpi.engine.LogEntry
import io.github.alexdev404.sonicpi.engine.LogKind
import io.github.alexdev404.sonicpi.ui.editor.BufferStrip
import io.github.alexdev404.sonicpi.ui.editor.CodeEditor
import io.github.alexdev404.sonicpi.ui.editor.KeyBar
import io.github.alexdev404.sonicpi.ui.editor.completion.CompletionEngine
import io.github.alexdev404.sonicpi.ui.editor.completion.DocSheet
import io.github.alexdev404.sonicpi.ui.editor.completion.SuggestionList
import io.github.alexdev404.sonicpi.ui.editor.completion.WordInfo
import io.github.alexdev404.sonicpi.ui.editor.completion.WordStrip
import io.github.alexdev404.sonicpi.ui.editor.completion.accept
import io.github.alexdev404.sonicpi.ui.editor.completion.info
import io.github.alexdev404.sonicpi.ui.editor.lineNumberAt
import io.github.alexdev404.sonicpi.ui.learn.LearnScreen
import io.github.alexdev404.sonicpi.ui.log.CuesView
import io.github.alexdev404.sonicpi.ui.log.LogView
import io.github.alexdev404.sonicpi.ui.log.PaneTitle
import io.github.alexdev404.sonicpi.ui.theme.LocalCodeFont
import io.github.alexdev404.sonicpi.ui.theme.LocalToolbar
import io.github.alexdev404.sonicpi.ui.theme.SonicPiColors

/** What fills the screen first: the code, the log in full (on a phone), or the help. */
enum class Destination { Code, Log, Learn }

/** Everything the screens show. */
data class AppUiState(
    val status: EngineStatus,
    val running: Boolean,
    val log: List<LogEntry>,
    val buffers: List<TextFieldValue>,
    val current: Int,
    val errorLine: Int,
    val fontSize: Int,
    val functions: Set<String>,
    val sections: List<LibrarySection>,
    val canUndo: Boolean,
    val canRedo: Boolean,
    /** Sonic Pi's completion, once its data is read. */
    val completion: CompletionEngine? = null,
)

/** Everything the screens can ask for. */
data class AppActions(
    val run: () -> Unit = {},
    val stop: () -> Unit = {},
    val selectBuffer: (Int) -> Unit = {},
    val edit: (TextFieldValue) -> Unit = {},
    val insert: (String) -> Unit = {},
    val undo: () -> Unit = {},
    val redo: () -> Unit = {},
    val openCode: (String) -> Unit = {},
    val play: (String) -> Unit = {},
    val clearLog: () -> Unit = {},
    val fontSize: (Int) -> Unit = {},
    val openFile: () -> Unit = {},
    val saveFile: () -> Unit = {},
)

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun SonicPiApp(state: AppUiState, actions: AppActions, initial: Destination = Destination.Code) {
    val p = SonicPiColors
    var help by rememberSaveable { mutableStateOf(initial == Destination.Learn) }
    var pane by rememberSaveable { mutableStateOf(if (initial == Destination.Log) PaneSize.Full else PaneSize.Normal) }
    val width = LocalConfiguration.current.screenWidthDp
    val desk = width >= 840            // the desktop's layout: the log beside the code, help under both
    val keyboard = WindowInsets.isImeVisible

    if (state.status is EngineStatus.Preparing) {
        BootScreen(state.status)
        return
    }

    Column(Modifier.fillMaxSize().background(p.Background).windowInsetsPadding(WindowInsets.safeDrawing)) {
        Toolbar(actions, help = help, onHelp = { help = !help }, sizes = width >= 600)
        if (desk) {
            Row(Modifier.weight(1f).fillMaxWidth()) {
                CodePane(state, actions, keyboard, Modifier.weight(0.62f))
                VerticalRule()
                Column(Modifier.weight(0.38f).fillMaxHeight()) {
                    LogView(state.log, Modifier.weight(2f).fillMaxWidth())
                    HorizontalRule()
                    CuesView(state.log, Modifier.weight(1f).fillMaxWidth())
                }
            }
            if (help) {
                HorizontalRule()
                HelpPanel(state, actions, onOpened = { help = false }, modifier = Modifier.weight(0.8f))
            }
        } else if (help) {
            HelpPanel(state, actions, onOpened = { help = false }, modifier = Modifier.weight(1f))
        } else {
            val showPane = !keyboard
            if (pane != PaneSize.Full || !showPane) CodePane(state, actions, keyboard, Modifier.weight(1f))
            if (showPane) {
                HorizontalRule()
                SidePane(
                    state.log, pane, onSize = { pane = it }, onClear = actions.clearLog,
                    modifier = when (pane) {
                        PaneSize.Full -> Modifier.weight(1f)
                        PaneSize.Normal -> Modifier.fillMaxHeight(0.3f)
                        PaneSize.Small -> Modifier
                    },
                )
            }
        }
    }
}

/** How much of a phone's screen the log takes: its title only, a third, or all of it. */
enum class PaneSize { Small, Normal, Full }

/**
 * The desktop's toolbar, its own buttons: run, stop, load and save on the
 * left; text size and help on the right (help pink while it is open).
 */
@Composable
private fun Toolbar(actions: AppActions, help: Boolean, onHelp: () -> Unit, sizes: Boolean) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ToolbarButton("run", "Run", actions.run)
        ToolbarButton("stop", "Stop", actions.stop)
        ToolbarButton("load", "Load a file into this buffer", actions.openFile)
        ToolbarButton("save", "Save this buffer as a file", actions.saveFile)
        Spacer(Modifier.weight(1f))
        if (sizes) {
            ToolbarButton("size-down", "Smaller text", { actions.fontSize(-1) })
            ToolbarButton("size-up", "Larger text", { actions.fontSize(+1) })
        }
        ToolbarButton(if (help) "help-toggled" else "help", if (help) "Hide help" else "Help", onHelp)
    }
}

@Composable
private fun ToolbarButton(name: String, description: String, onClick: () -> Unit) {
    val p = SonicPiColors
    val image = LocalToolbar.current[name]
    val height = 27.dp
    val mod = Modifier
        .height(height)
        .clickable(onClick = onClick)
        .semantics { contentDescription = description; role = Role.Button }
    if (image != null) {
        Image(image, contentDescription = null, contentScale = ContentScale.FillHeight, modifier = mod.width(height * 167f / 60f))
    } else {
        // The button drawn as the image draws it: the label on grey.
        Box(mod.background(p.Button).padding(horizontal = 8.dp), contentAlignment = Alignment.Center) {
            Text(name.substringBefore('-'), fontFamily = LocalCodeFont.current, fontSize = 13.sp, color = p.OnButton)
        }
    }
}

@Composable
private fun HorizontalRule() = Box(Modifier.fillMaxWidth().height(1.dp).background(SonicPiColors.Button.copy(alpha = 0.3f)))

@Composable
private fun VerticalRule() = Box(Modifier.width(1.dp).fillMaxHeight().background(SonicPiColors.Button.copy(alpha = 0.3f)))

/** The log and the cues on a phone, under the code: either one, a grip to size them. */
@Composable
private fun SidePane(log: List<LogEntry>, size: PaneSize, onSize: (PaneSize) -> Unit, onClear: () -> Unit, modifier: Modifier) {
    val p = SonicPiColors
    var cues by rememberSaveable { mutableStateOf(false) }
    Column(modifier.fillMaxWidth().background(p.Background)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            PaneTitle("Log", selected = !cues, onClick = { cues = false; if (size == PaneSize.Small) onSize(PaneSize.Normal) })
            PaneTitle("Cues", selected = cues, onClick = { cues = true; if (size == PaneSize.Small) onSize(PaneSize.Normal) })
            Spacer(Modifier.weight(1f))
            PaneTitle("Clear", selected = false, onClick = onClear)
            Grip("⌃", "Larger log", enabled = size != PaneSize.Full) { onSize(if (size == PaneSize.Small) PaneSize.Normal else PaneSize.Full) }
            Grip("⌄", "Smaller log", enabled = size != PaneSize.Small) { onSize(if (size == PaneSize.Full) PaneSize.Normal else PaneSize.Small) }
        }
        if (size != PaneSize.Small) {
            if (cues) CuesView(log, Modifier.weight(1f).fillMaxWidth(), title = false)
            else LogView(log, Modifier.weight(1f).fillMaxWidth(), title = false)
        }
    }
}

/** The desktop's up and down grips, at a pane's top right. */
@Composable
private fun Grip(glyph: String, description: String, enabled: Boolean, onClick: () -> Unit) {
    val p = SonicPiColors
    Box(
        Modifier.size(36.dp).clickable(enabled = enabled, onClick = onClick).semantics { contentDescription = description },
        contentAlignment = Alignment.Center,
    ) {
        Text(glyph, fontSize = 18.sp, color = if (enabled) p.Muted else p.Muted.copy(alpha = 0.3f))
    }
}

@Composable
private fun HelpPanel(state: AppUiState, actions: AppActions, onOpened: () -> Unit, modifier: Modifier) {
    val narrow = LocalConfiguration.current.screenWidthDp < 840
    LearnScreen(
        state.sections, state.functions,
        // On a phone the code comes back into view with what was opened in it.
        onOpenCode = { actions.openCode(it); if (narrow) onOpened() },
        onPlay = actions.play,
        modifier = modifier,
    )
}

@Composable
private fun CodePane(state: AppUiState, actions: AppActions, keyboard: Boolean, modifier: Modifier) {
    val p = SonicPiColors
    Column(modifier.fillMaxHeight()) {
        if (state.status is EngineStatus.Failed) {
            Banner("Sonic Pi could not start: ${state.status.message}")
        }
        val lastError = state.log.lastOrNull { it.kind == LogKind.Error }
        AnimatedVisibility(visible = state.errorLine > 0 && lastError != null) {
            Banner("Line ${state.errorLine}: ${lastError?.text.orEmpty()}")
        }
        val value = state.buffers[state.current]
        // Offered as code is typed, as Sonic Pi's editor offers it: not when the caret is only moved.
        var typing by remember { mutableStateOf(false) }
        var sheet by remember { mutableStateOf<WordInfo?>(null) }
        val engine = state.completion
        val caret = value.selection.start
        val suggestions = remember(engine, value.text, caret, typing) {
            if (engine == null || !typing || !value.selection.collapsed) null else engine.suggest(value.text, caret)
        }
        val word = remember(engine, value.text, caret) {
            if (engine == null || !value.selection.collapsed) null else engine.infoAt(value.text, caret)
        }
        CodeEditor(
            value = value,
            onValueChange = { typing = it.text != value.text; actions.edit(it) },
            functions = state.functions,
            fontSize = state.fontSize.sp,
            errorLine = state.errorLine,
            modifier = Modifier.weight(1f).fillMaxWidth(),
        )
        when {
            suggestions != null -> SuggestionList(
                suggestions,
                onAccept = { item -> typing = true; actions.edit(accept(value, suggestions, item)) },
                onInfo = { sheet = it.info() },
            )
            word != null -> WordStrip(word, onOpen = { sheet = word })
        }
        sheet?.let { DocSheet(it, state.functions, onDismiss = { sheet = null }) }
        val position = remember(value.text, caret) {
            val lineStart = value.text.lastIndexOf('\n', caret - 1) + 1
            value.text.lineNumberAt(caret) to (caret - lineStart + 1)
        }
        BufferStrip(state.buffers.size, state.current, position, onSelect = actions.selectBuffer)
        if (keyboard) {
            KeyBar(onInsert = { typing = true; actions.insert(it) }, onUndo = actions.undo, onRedo = actions.redo,
                canUndo = state.canUndo, canRedo = state.canRedo)
        }
    }
}

/** An error, over the code: red on the theme's ground, with a red bar beside it. */
@Composable
private fun Banner(text: String) {
    val p = SonicPiColors
    Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min).background(p.Red.copy(alpha = if (p.dark) 0.18f else 0.08f))) {
        Box(Modifier.width(4.dp).fillMaxHeight().background(p.Red))
        Text(text, fontFamily = LocalCodeFont.current, fontSize = 13.sp, color = if (p.dark) Color.White else p.Red,
            maxLines = 3, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
    }
}

/** Starting: Sonic Pi's logo, and how far the sounds are unpacked. */
@Composable
private fun BootScreen(status: EngineStatus.Preparing) {
    val p = SonicPiColors
    val assets = LocalContext.current.assets
    val logo = remember(assets, p.dark) {
        runCatching { assets.open("sonicpi/gui/logo-${if (p.dark) "dark" else "light"}.png").use { BitmapFactory.decodeStream(it)?.asImageBitmap() } }.getOrNull()
    }
    Column(
        Modifier.fillMaxSize().background(p.Background).padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        if (logo != null) {
            Image(logo, contentDescription = "Sonic Pi", modifier = Modifier.fillMaxWidth(0.7f))
        } else {
            Text("Sonic Pi", fontSize = 32.sp, fontWeight = FontWeight.Bold, color = p.Foreground)
        }
        Spacer(Modifier.height(32.dp))
        Box(Modifier.fillMaxWidth(0.7f).height(4.dp).background(p.Base)) {
            Box(Modifier.fillMaxWidth(status.progress.coerceIn(0f, 1f)).fillMaxHeight().background(p.Pink))
        }
        Spacer(Modifier.height(12.dp))
        Text("${status.message}…", fontFamily = LocalCodeFont.current, fontSize = 13.sp, color = p.Muted)
    }
}
