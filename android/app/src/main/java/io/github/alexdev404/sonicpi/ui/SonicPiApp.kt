// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui

import android.graphics.BitmapFactory
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import io.github.alexdev404.sonicpi.R
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
import io.github.alexdev404.sonicpi.ui.editor.CaretStatus
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
import io.github.alexdev404.sonicpi.ui.theme.SonicPiColors

/** The app's three places, as tabs at the bottom (a rail on a tablet). */
enum class Destination(val label: String, val icon: Int) {
    Code("code", R.drawable.ic_code),
    Log("log", R.drawable.ic_log),
    Learn("help", R.drawable.ic_help),
}

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
    var destination by rememberSaveable { mutableStateOf(initial) }
    val width = LocalConfiguration.current.screenWidthDp
    val wide = width >= 600           // the tabs as a rail beside the content
    val split = width >= 840          // the log beside the code, as the desktop's side column
    val keyboard = WindowInsets.isImeVisible

    if (state.status is EngineStatus.Preparing) {
        BootScreen(state.status)
        return
    }

    Column(Modifier.fillMaxSize().background(p.Bar).windowInsetsPadding(WindowInsets.safeDrawing)) {
        TopBar(state, actions)
        Row(Modifier.weight(1f).fillMaxWidth()) {
            if (wide) Rail(destination) { destination = it }
            Box(Modifier.weight(1f).fillMaxHeight().background(p.Background)) {
                when (destination) {
                    Destination.Code -> Row(Modifier.fillMaxSize()) {
                        CodePane(state, actions, peek = !split && !keyboard, onPeek = { destination = Destination.Log },
                            modifier = Modifier.weight(if (split) 0.62f else 1f))
                        if (split) {
                            VerticalRule()
                            Column(Modifier.weight(0.38f).fillMaxHeight()) {
                                LogView(state.log, Modifier.weight(2f).fillMaxWidth())
                                HorizontalRule()
                                CuesView(state.log, Modifier.weight(1f).fillMaxWidth())
                            }
                        }
                    }
                    Destination.Log -> LogAndCues(state.log, actions.clearLog, Modifier.fillMaxSize())
                    Destination.Learn -> LearnScreen(
                        state.sections, state.functions,
                        onOpenCode = { actions.openCode(it); destination = Destination.Code },
                        onPlay = actions.play,
                    )
                }
            }
        }
        if (!wide && !(keyboard && destination == Destination.Code)) BottomTabs(destination) { destination = it }
    }
}

/**
 * The top bar: Sonic Pi's icon and name, and the web version's toolbar glyphs, bare:
 * run (pink while a program plays), stop, load and save; a menu for the rest.
 */
@Composable
private fun TopBar(state: AppUiState, actions: AppActions) {
    val p = SonicPiColors
    val ready = state.status == EngineStatus.Ready
    var menu by remember { mutableStateOf(false) }
    Row(
        Modifier.fillMaxWidth().background(p.Bar).padding(start = 16.dp, end = 4.dp, top = 6.dp, bottom = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        AppIcon(Modifier.size(30.dp))
        Spacer(Modifier.width(10.dp))
        Text("Sonic Pi", fontSize = 21.sp, fontWeight = FontWeight.Bold, color = p.Foreground)
        Spacer(Modifier.weight(1f))
        GlyphButton("run", R.drawable.ic_play, "Run", if (state.running) p.Pink else p.Foreground, ready, actions.run)
        GlyphButton("stop", R.drawable.ic_stop, "Stop", p.Foreground, ready, actions.stop)
        GlyphButton("load", R.drawable.ic_more, "Open a file", p.Foreground, true, actions.openFile)
        GlyphButton("save", R.drawable.ic_more, "Save this buffer as a file", p.Foreground, true, actions.saveFile)
        Box {
            IconButton(onClick = { menu = true }, modifier = Modifier.semantics { contentDescription = "More" }) {
                Icon(painterResource(R.drawable.ic_more), contentDescription = null, tint = p.Muted)
            }
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }, containerColor = p.Base) {
                DropdownMenuItem(text = { Text("Larger text") }, onClick = { actions.fontSize(+1) })
                DropdownMenuItem(text = { Text("Smaller text") }, onClick = { actions.fontSize(-1) })
                HorizontalDivider(color = p.Border)
                DropdownMenuItem(text = { Text("Clear log") }, onClick = { menu = false; actions.clearLog() })
            }
        }
    }
}

/** Sonic Pi's own icon (app/web/web/data/icon-256.png), as the app's mark. */
@Composable
private fun AppIcon(modifier: Modifier) {
    val assets = LocalContext.current.assets
    val icon = remember(assets) {
        runCatching { assets.open("sonicpi/gui/icon.png").use { BitmapFactory.decodeStream(it)?.asImageBitmap() } }.getOrNull()
    }
    if (icon != null) Image(icon, contentDescription = null, modifier = modifier)
}

@Composable
private fun GlyphButton(name: String, fallback: Int, description: String, tint: Color, enabled: Boolean, onClick: () -> Unit) {
    Box(
        Modifier.size(48.dp).clip(CircleShape).clickable(enabled = enabled, onClick = onClick)
            .semantics { contentDescription = description; role = Role.Button },
        contentAlignment = Alignment.Center,
    ) {
        Glyph(name, if (enabled) tint else tint.copy(alpha = 0.35f), fallback, Modifier.size(24.dp))
    }
}

/** A tab's icon: on a pink square when chosen, as Sonic Pi shows a toggled one. */
@Composable
private fun TabIcon(d: Destination, selected: Boolean) {
    val p = SonicPiColors
    Box(
        Modifier.size(width = 52.dp, height = 30.dp).clip(RoundedCornerShape(4.dp)).background(if (selected) p.Pink else Color.Transparent),
        contentAlignment = Alignment.Center,
    ) {
        val tint = if (selected) p.OnButton else p.Muted
        if (d == Destination.Learn) Glyph("help", tint, d.icon, Modifier.size(20.dp))
        else Icon(painterResource(d.icon), contentDescription = null, tint = tint, modifier = Modifier.size(22.dp))
    }
}

@Composable
private fun BottomTabs(selected: Destination, onSelect: (Destination) -> Unit) {
    val p = SonicPiColors
    Column {
        Box(Modifier.fillMaxWidth().height(1.dp).background(p.Border))
        Row(Modifier.fillMaxWidth().background(p.Bar).padding(vertical = 6.dp)) {
            for (d in Destination.entries) {
                val on = d == selected
                Column(
                    Modifier.weight(1f).clickable { onSelect(d) }.padding(vertical = 4.dp)
                        .semantics { contentDescription = d.label + if (on) ", selected" else ""; role = Role.Tab },
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    TabIcon(d, on)
                    Spacer(Modifier.height(4.dp))
                    Text(d.label, fontFamily = LocalCodeFont.current, fontSize = 12.sp, color = if (on) p.Pink else p.Muted)
                }
            }
        }
    }
}

@Composable
private fun Rail(selected: Destination, onSelect: (Destination) -> Unit) {
    val p = SonicPiColors
    Row {
        Column(Modifier.fillMaxHeight().width(80.dp).background(p.Bar).padding(top = 8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            for (d in Destination.entries) {
                val on = d == selected
                Column(
                    Modifier.fillMaxWidth().clickable { onSelect(d) }.padding(vertical = 10.dp)
                        .semantics { contentDescription = d.label + if (on) ", selected" else ""; role = Role.Tab },
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    TabIcon(d, on)
                    Spacer(Modifier.height(4.dp))
                    Text(d.label, fontFamily = LocalCodeFont.current, fontSize = 12.sp, color = if (on) p.Pink else p.Muted)
                }
            }
        }
        Box(Modifier.width(1.dp).fillMaxHeight().background(p.Border))
    }
}

@Composable
private fun HorizontalRule() = Box(Modifier.fillMaxWidth().height(1.dp).background(SonicPiColors.Border))

@Composable
private fun VerticalRule() = Box(Modifier.width(1.dp).fillMaxHeight().background(SonicPiColors.Border))

/** The log tab: the log or the cues, picked by their titles, as the desktop's side column names them. */
@Composable
private fun LogAndCues(log: List<LogEntry>, onClear: () -> Unit, modifier: Modifier) {
    var cues by rememberSaveable { mutableStateOf(false) }
    Column(modifier.background(SonicPiColors.Background)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            PaneTitle("Log", selected = !cues, onClick = { cues = false })
            PaneTitle("Cues", selected = cues, onClick = { cues = true })
            Spacer(Modifier.weight(1f))
            PaneTitle("Clear", selected = false, onClick = onClear)
        }
        if (cues) CuesView(log, Modifier.weight(1f).fillMaxWidth(), title = false)
        else LogView(log, Modifier.weight(1f).fillMaxWidth(), title = false)
    }
}

/** The log's last lines under the code, on a phone; a tap opens the log. */
@Composable
private fun LogPeek(log: List<LogEntry>, onClick: () -> Unit) {
    val p = SonicPiColors
    val last = log.filter { it.kind != LogKind.Cue }.takeLast(2)
    Column(
        Modifier.fillMaxWidth().background(p.Background).clickable(onClick = onClick).padding(horizontal = 12.dp, vertical = 6.dp)
            .semantics { contentDescription = "The log's last lines. Open the log" },
    ) {
        if (last.isEmpty()) PaneTitle("Log", modifier = Modifier.padding(0.dp))
        for (e in last) {
            Text(
                e.text, maxLines = 1, overflow = TextOverflow.Ellipsis, fontFamily = LocalCodeFont.current, fontSize = 12.sp,
                color = when {
                    e.kind == LogKind.Error -> p.Red
                    e.text.startsWith("sample") -> p.Blue
                    else -> p.Foreground
                },
            )
        }
    }
}

@Composable
private fun CodePane(state: AppUiState, actions: AppActions, peek: Boolean, onPeek: () -> Unit, modifier: Modifier) {
    Column(modifier.fillMaxHeight()) {
        BufferStrip(state.buffers.size, state.current, onSelect = actions.selectBuffer)
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
        CaretStatus(position.first, position.second)
        if (peek && suggestions == null && word == null) LogPeek(state.log, onPeek)
        KeyBar(onInsert = { typing = true; actions.insert(it) }, onUndo = actions.undo, onRedo = actions.redo,
            canUndo = state.canUndo, canRedo = state.canRedo)
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
