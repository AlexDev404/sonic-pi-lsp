// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.log

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.alexdev404.sonicpi.engine.LogEntry
import io.github.alexdev404.sonicpi.engine.LogKind
import io.github.alexdev404.sonicpi.ui.theme.LocalCodeFont
import io.github.alexdev404.sonicpi.ui.theme.SonicPiColors

/** What the log shows: everything a program says, or only its cues. */
enum class LogFilter(val label: String) { All("Log"), Cues("Cues"), Errors("Errors") }

/**
 * The log, as Sonic Pi lays it out: a header for each moment of each run,
 * {run: 1, time: 2.5}, with what happened then beneath it.
 */
@Composable
fun LogView(entries: List<LogEntry>, modifier: Modifier = Modifier, compact: Boolean = false) {
    var filter by rememberSaveable { mutableStateOf(LogFilter.All) }
    val shown = remember(entries, filter) {
        when (filter) {
            LogFilter.All -> entries.filter { it.kind != LogKind.Cue }
            LogFilter.Cues -> entries.filter { it.kind == LogKind.Cue }
            LogFilter.Errors -> entries.filter { it.kind == LogKind.Error || it.kind == LogKind.Engine }
        }
    }
    Column(modifier.background(SonicPiColors.Black)) {
        if (!compact) {
            Row(Modifier.padding(horizontal = 12.dp, vertical = 6.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                for (f in LogFilter.entries) {
                    FilterChip(selected = f == filter, onClick = { filter = f }, label = { Text(f.label) })
                }
            }
        }
        val state = rememberLazyListState()
        // Follow the newest line, as Sonic Pi's log does.
        LaunchedEffect(shown.size) { if (shown.isNotEmpty()) state.scrollToItem(shown.lastIndex) }
        if (shown.isEmpty()) {
            Box(Modifier.fillMaxSize().padding(24.dp)) {
                Text(
                    if (filter == LogFilter.Cues) "Cues appear here: every live_loop sends one each time round."
                    else "Press Run: what your program plays and says appears here.",
                    color = SonicPiColors.Dim,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        } else {
            LazyColumn(state = state, modifier = Modifier.fillMaxSize().padding(horizontal = 10.dp)) {
                itemsIndexed(shown, key = { _, e -> e.id }) { index, entry ->
                    val prev = shown.getOrNull(index - 1)
                    val newMoment = prev == null || prev.job != entry.job || kotlin.math.abs(prev.time - entry.time) > 0.0005
                    LogLine(entry, header = newMoment && entry.job > 0)
                }
            }
        }
    }
}

@Composable
private fun LogLine(entry: LogEntry, header: Boolean) {
    val code = LocalCodeFont.current
    Column(Modifier.fillMaxWidth().padding(vertical = 1.dp)) {
        if (header) {
            Text(
                "{run: ${entry.job}, time: ${"%.4f".format(entry.time).trimEnd('0').trimEnd('.')}${if (entry.thread.isNotEmpty()) ", thread: :${entry.thread}" else ""}}",
                fontFamily = code, fontSize = 12.sp, color = SonicPiColors.Pink,
                modifier = Modifier.padding(top = 6.dp),
            )
        }
        val (fg, bg) = colors(entry.kind)
        Text(
            text = (if (entry.job > 0 && entry.kind != LogKind.State) " └─ " else "") + entry.text +
                (if (entry.kind == LogKind.Error && entry.line > 0) "  (line ${entry.line})" else ""),
            fontFamily = code,
            fontSize = 13.sp,
            fontWeight = if (entry.kind == LogKind.Error) FontWeight.Bold else FontWeight.Normal,
            color = fg,
            maxLines = 6,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier
                .background(bg, RoundedCornerShape(4.dp))
                .padding(horizontal = if (bg == Color.Transparent) 0.dp else 6.dp, vertical = 1.dp),
        )
    }
}

private fun colors(kind: LogKind): Pair<Color, Color> = when (kind) {
    LogKind.Output -> SonicPiColors.Blue to Color.Transparent
    LogKind.Error -> Color.White to SonicPiColors.Red.copy(alpha = 0.55f)
    LogKind.Engine -> SonicPiColors.Orange to Color.Transparent
    LogKind.Cue -> SonicPiColors.Yellow to Color.Transparent
    LogKind.State -> SonicPiColors.Dim to Color.Transparent
    LogKind.Log -> SonicPiColors.Dim to Color.Transparent
    LogKind.Synth -> SonicPiColors.Text to Color.Transparent
}
