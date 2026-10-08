// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.log

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.alexdev404.sonicpi.engine.LogEntry
import io.github.alexdev404.sonicpi.engine.LogKind
import io.github.alexdev404.sonicpi.ui.theme.LocalCodeFont
import io.github.alexdev404.sonicpi.ui.theme.SonicPiColors
import io.github.alexdev404.sonicpi.ui.theme.SonicPiPalette

/** A pane's title, as the desktop's side column has them: small, grey, upper case. */
@Composable
fun PaneTitle(text: String, modifier: Modifier = Modifier, selected: Boolean = true, onClick: (() -> Unit)? = null) {
    val p = SonicPiColors
    Text(
        text.uppercase(),
        fontFamily = LocalCodeFont.current, fontSize = 11.sp, letterSpacing = 0.5.sp,
        color = if (selected) p.Muted else p.Muted.copy(alpha = 0.45f),
        modifier = modifier
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
            .padding(horizontal = 8.dp, vertical = 6.dp),
    )
}

/** One moment of one run: its header and what happened then. */
private data class Moment(val key: Long, val header: String?, val lines: List<LogEntry>)

private fun moments(entries: List<LogEntry>): List<Moment> {
    val out = ArrayList<Moment>()
    var cur = ArrayList<LogEntry>()
    var head: LogEntry? = null
    fun flush() {
        val h = head ?: return
        out += Moment(h.id, if (h.job > 0) header(h) else null, cur)
        cur = ArrayList()
        head = null
    }
    for (e in entries) {
        val h = head
        if (h == null || h.job != e.job || kotlin.math.abs(h.time - e.time) > 0.0005 || h.thread != e.thread || e.job == 0) {
            flush()
            head = e
        }
        cur += e
    }
    flush()
    return out
}

private fun header(e: LogEntry) =
    "{run: ${e.job}, time: ${"%.4f".format(e.time).trimEnd('0').trimEnd('.')}" +
        (if (e.thread.isNotEmpty()) ", thread: :${e.thread}" else "") + "}"

/** A line's colours, as the desktop's log streams have them (LogForeground_n / LogBackground_n). */
private fun colours(e: LogEntry, p: SonicPiPalette): Pair<Color, Color> = when (e.kind) {
    LogKind.Synth -> (if (e.text.startsWith("sample")) p.Blue else p.Foreground) to Color.Transparent
    LogKind.Output -> p.Foreground to Color.Transparent
    LogKind.Cue -> Color.White to p.Pink
    LogKind.Error -> Color.White to p.Red
    LogKind.Engine -> Color.White to p.Orange
    LogKind.State, LogKind.Log -> p.Muted to Color.Transparent
}

/**
 * The log, as Sonic Pi lays it out: a header for each moment of each run,
 * {run: 1, time: 2.5, thread: :drums}, and what happened then beneath it,
 * as branches (├─, └─). The newest is followed.
 */
@Composable
fun LogView(entries: List<LogEntry>, modifier: Modifier = Modifier, title: Boolean = true) {
    val p = SonicPiColors
    val code = LocalCodeFont.current
    val shown = remember(entries) { moments(entries.filter { it.kind != LogKind.Cue }) }
    Column(modifier.background(p.Background)) {
        if (title) PaneTitle("Log")
        val state = rememberLazyListState()
        LaunchedEffect(shown.size, shown.lastOrNull()?.lines?.size) { if (shown.isNotEmpty()) state.scrollToItem(shown.lastIndex) }
        if (shown.isEmpty()) {
            Text(
                "Press Run: what your program plays and says appears here.",
                color = p.Muted, fontSize = 13.sp, modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
            )
            return@Column
        }
        LazyColumn(state = state, modifier = Modifier.fillMaxSize().padding(horizontal = 8.dp).semantics { contentDescription = "Log" }) {
            items(shown, key = { it.key }) { m ->
                val text = buildAnnotatedString {
                    m.header?.let { append(it); append('\n') }
                    m.lines.forEachIndexed { i, e ->
                        if (m.header != null) withStyle(SpanStyle(color = p.Foreground)) { append(if (i == m.lines.lastIndex) " └─ " else " ├─ ") }
                        val (fg, bg) = colours(e, p)
                        withStyle(SpanStyle(color = fg, background = bg)) {
                            append(e.text)
                            if (e.kind == LogKind.Error && e.line > 0) append("  (line ${e.line})")
                        }
                        if (i < m.lines.lastIndex) append('\n')
                    }
                }
                Text(text, fontFamily = code, fontSize = 12.sp, lineHeight = 17.sp, color = p.Foreground, modifier = Modifier.padding(bottom = 8.dp))
            }
        }
    }
}

/** The cues: each path a program has cued (every live_loop cues its name), newest last. */
@Composable
fun CuesView(entries: List<LogEntry>, modifier: Modifier = Modifier, title: Boolean = true) {
    val p = SonicPiColors
    val cues = remember(entries) { entries.filter { it.kind == LogKind.Cue }.map { it.text }.distinct().takeLast(64) }
    Column(modifier.background(p.Background)) {
        if (title) PaneTitle("Cues")
        LazyColumn(Modifier.fillMaxSize().padding(horizontal = 8.dp).semantics { contentDescription = "Cues" }) {
            items(cues, key = { it }) { path ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(path, fontFamily = LocalCodeFont.current, fontSize = 12.sp, color = p.Foreground)
                    Spacer(Modifier.width(16.dp))
                    Text("[]", fontFamily = LocalCodeFont.current, fontSize = 12.sp, color = p.Foreground)
                }
            }
        }
    }
}

/** A hairline between panes (WindowBorder). */
@Composable
fun PaneRule(modifier: Modifier = Modifier) {
    Box(modifier.fillMaxWidth().padding(vertical = 0.dp).background(SonicPiColors.Button.copy(alpha = 0.35f)).padding(top = 1.dp))
}
