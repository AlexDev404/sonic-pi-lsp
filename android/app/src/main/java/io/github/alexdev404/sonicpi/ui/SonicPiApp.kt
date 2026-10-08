// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.isImeVisible
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.NavigationRailItemDefaults
import androidx.compose.material3.OutlinedIconButton
import androidx.compose.material3.PrimaryScrollableTabRow
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.alexdev404.sonicpi.R
import io.github.alexdev404.sonicpi.data.LibrarySection
import io.github.alexdev404.sonicpi.engine.EngineStatus
import io.github.alexdev404.sonicpi.engine.LogEntry
import io.github.alexdev404.sonicpi.engine.LogKind
import io.github.alexdev404.sonicpi.ui.editor.CodeEditor
import io.github.alexdev404.sonicpi.ui.editor.KeyBar
import io.github.alexdev404.sonicpi.ui.editor.completion.CompletionEngine
import io.github.alexdev404.sonicpi.ui.editor.completion.DocSheet
import io.github.alexdev404.sonicpi.ui.editor.completion.SuggestionList
import io.github.alexdev404.sonicpi.ui.editor.completion.WordInfo
import io.github.alexdev404.sonicpi.ui.editor.completion.WordStrip
import io.github.alexdev404.sonicpi.ui.editor.completion.accept
import io.github.alexdev404.sonicpi.ui.editor.completion.info
import io.github.alexdev404.sonicpi.ui.learn.LearnScreen
import io.github.alexdev404.sonicpi.ui.log.LogView
import io.github.alexdev404.sonicpi.ui.theme.LocalCodeFont
import io.github.alexdev404.sonicpi.ui.theme.SonicPiColors

enum class Destination(val label: String, val icon: Int) {
    Code("Code", R.drawable.ic_code),
    Log("Log", R.drawable.ic_log),
    Learn("Learn", R.drawable.ic_learn),
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
    var destination by rememberSaveable { mutableStateOf(initial) }
    val width = LocalConfiguration.current.screenWidthDp
    val wide = width >= 600           // a rail beside the content
    val split = width >= 840          // the log beside the editor
    val keyboard = WindowInsets.isImeVisible

    if (state.status is EngineStatus.Preparing) {
        BootScreen(state.status)
        return
    }

    Scaffold(
        containerColor = SonicPiColors.Black,
        topBar = { TopBar(state, actions, destination) },
        bottomBar = {
            if (!wide && !(keyboard && destination == Destination.Code)) {
                NavigationBar(containerColor = MaterialTheme.colorScheme.surfaceContainer) {
                    for (d in Destination.entries) {
                        NavigationBarItem(
                            selected = d == destination,
                            onClick = { destination = d },
                            icon = { Icon(painterResource(d.icon), contentDescription = null) },
                            label = { Text(d.label) },
                            colors = NavigationBarItemDefaults.colors(
                                indicatorColor = MaterialTheme.colorScheme.primaryContainer,
                                selectedIconColor = Color.White, selectedTextColor = SonicPiColors.Pink,
                            ),
                        )
                    }
                }
            }
        },
    ) { padding ->
        Row(Modifier.fillMaxSize().padding(padding).consumeWindowInsets(padding)) {
            if (wide) {
                NavigationRail(containerColor = MaterialTheme.colorScheme.surfaceContainer) {
                    Spacer(Modifier.height(8.dp))
                    for (d in Destination.entries) {
                        NavigationRailItem(
                            selected = d == destination,
                            onClick = { destination = d },
                            icon = { Icon(painterResource(d.icon), contentDescription = null) },
                            label = { Text(d.label) },
                            colors = NavigationRailItemDefaults.colors(
                                indicatorColor = MaterialTheme.colorScheme.primaryContainer,
                                selectedIconColor = Color.White, selectedTextColor = SonicPiColors.Pink,
                            ),
                        )
                    }
                }
            }
            Box(Modifier.weight(1f).fillMaxHeight()) {
                when (destination) {
                    Destination.Code -> Row(Modifier.fillMaxSize()) {
                        CodePane(state, actions, showPeek = !split && !keyboard, onPeek = { destination = Destination.Log },
                            modifier = Modifier.weight(if (split) 0.62f else 1f))
                        if (split) {
                            VerticalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                            LogView(state.log, Modifier.weight(0.38f))
                        }
                    }
                    Destination.Log -> LogView(state.log, Modifier.fillMaxSize())
                    Destination.Learn -> LearnScreen(
                        state.sections, state.functions,
                        onOpenCode = { actions.openCode(it); destination = Destination.Code },
                        onPlay = actions.play,
                    )
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TopBar(state: AppUiState, actions: AppActions, destination: Destination) {
    var menu by remember { mutableStateOf(false) }
    val ready = state.status == EngineStatus.Ready
    TopAppBar(
        colors = TopAppBarDefaults.topAppBarColors(containerColor = SonicPiColors.Black),
        title = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Sonic Pi", fontWeight = FontWeight.Bold)
                if (state.running) {
                    Spacer(Modifier.width(10.dp))
                    PlayingDot()
                }
            }
        },
        actions = {
            OutlinedIconButton(onClick = actions.stop, enabled = ready && state.running) {
                Icon(painterResource(R.drawable.ic_stop), contentDescription = "Stop")
            }
            Spacer(Modifier.width(8.dp))
            Button(
                onClick = actions.run,
                enabled = ready,
                colors = ButtonDefaults.buttonColors(containerColor = SonicPiColors.Pink),
            ) {
                Icon(painterResource(R.drawable.ic_play), contentDescription = null)
                Spacer(Modifier.width(4.dp))
                Text("Run")
            }
            IconButton(onClick = { menu = true }) { Icon(painterResource(R.drawable.ic_more), contentDescription = "More") }
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                DropdownMenuItem(text = { Text("Open file…") }, onClick = { menu = false; actions.openFile() })
                DropdownMenuItem(text = { Text("Save buffer as…") }, onClick = { menu = false; actions.saveFile() })
                HorizontalDivider()
                DropdownMenuItem(text = { Text("Larger text") }, onClick = { actions.fontSize(+1) })
                DropdownMenuItem(text = { Text("Smaller text") }, onClick = { actions.fontSize(-1) })
                HorizontalDivider()
                DropdownMenuItem(text = { Text("Clear log") }, onClick = { menu = false; actions.clearLog() })
            }
        },
    )
}

@Composable
private fun PlayingDot() {
    val pulse by rememberInfiniteTransition(label = "playing").animateFloat(
        initialValue = 0.35f, targetValue = 1f, label = "pulse",
        animationSpec = infiniteRepeatable(tween(600), RepeatMode.Reverse),
    )
    Box(Modifier.size(10.dp).alpha(pulse).background(SonicPiColors.Pink, CircleShape))
}

@Composable
private fun CodePane(state: AppUiState, actions: AppActions, showPeek: Boolean, onPeek: () -> Unit, modifier: Modifier) {
    Column(modifier.fillMaxHeight().imePadding()) {
        PrimaryScrollableTabRow(selectedTabIndex = state.current, edgePadding = 4.dp, containerColor = SonicPiColors.Black, minTabWidth = 48.dp) {
            state.buffers.forEachIndexed { i, b ->
                Tab(
                    selected = i == state.current,
                    onClick = { actions.selectBuffer(i) },
                    selectedContentColor = SonicPiColors.Pink,
                    unselectedContentColor = SonicPiColors.Text,
                    text = { Text(if (b.text.isBlank() && i != state.current) "$i" else "$i", fontFamily = LocalCodeFont.current) },
                    modifier = Modifier.alpha(if (b.text.isBlank() && i != state.current) 0.55f else 1f),
                )
            }
        }
        if (state.status is EngineStatus.Failed) {
            Banner("Sonic Pi could not start: ${state.status.message}", error = true)
        }
        val lastError = state.log.lastOrNull { it.kind == LogKind.Error }
        AnimatedVisibility(visible = state.errorLine > 0 && lastError != null) {
            Banner("Line ${state.errorLine}: ${lastError?.text.orEmpty()}", error = true)
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
            showPeek -> LogPeek(state.log, onPeek)
        }
        sheet?.let { DocSheet(it, state.functions, onDismiss = { sheet = null }) }
        KeyBar(onInsert = { typing = true; actions.insert(it) }, onUndo = actions.undo, onRedo = actions.redo, canUndo = state.canUndo, canRedo = state.canRedo)
    }
}

@Composable
private fun Banner(text: String, error: Boolean) {
    Surface(color = if (error) MaterialTheme.colorScheme.errorContainer else MaterialTheme.colorScheme.secondaryContainer) {
        Text(text, style = MaterialTheme.typography.bodyMedium, maxLines = 3, overflow = TextOverflow.Ellipsis,
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp))
    }
}

/** The log's last lines under the editor, on a phone: a tap opens the whole log. */
@Composable
private fun LogPeek(log: List<LogEntry>, onClick: () -> Unit) {
    val last = log.filter { it.kind != LogKind.Cue }.takeLast(2)
    Column(
        Modifier.fillMaxWidth().background(SonicPiColors.Black).clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 6.dp),
    ) {
        if (last.isEmpty()) {
            Text("Log", color = SonicPiColors.Grey, fontSize = 12.sp)
        }
        for (e in last) {
            Text(
                e.text, maxLines = 1, overflow = TextOverflow.Ellipsis, fontFamily = LocalCodeFont.current, fontSize = 12.sp,
                color = when (e.kind) { LogKind.Error -> SonicPiColors.Red; LogKind.Output -> SonicPiColors.Blue; else -> SonicPiColors.Dim },
            )
        }
    }
}

@Composable
private fun BootScreen(status: EngineStatus.Preparing) {
    Column(
        Modifier.fillMaxSize().background(SonicPiColors.Black).padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(painterResource(R.drawable.ic_launcher_foreground), contentDescription = null, tint = SonicPiColors.Pink, modifier = Modifier.size(120.dp))
        Text("Sonic Pi", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.Bold, color = SonicPiColors.Text)
        Spacer(Modifier.height(24.dp))
        LinearProgressIndicator(
            progress = { status.progress },
            color = SonicPiColors.Pink,
            trackColor = SonicPiColors.Raised,
            modifier = Modifier.fillMaxWidth(0.7f).height(6.dp).background(SonicPiColors.Raised, RoundedCornerShape(3.dp)),
        )
        Spacer(Modifier.height(12.dp))
        Text("${status.message}…", color = SonicPiColors.Dim)
    }
}
