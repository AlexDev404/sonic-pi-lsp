// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.editor.completion

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.fromHtml
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.alexdev404.sonicpi.ui.editor.Highlighter
import io.github.alexdev404.sonicpi.ui.theme.LocalCodeFont
import io.github.alexdev404.sonicpi.ui.theme.SonicPiColors

/** A kind's colour: the colour the code gives what it inserts (cm.js's badges). */
internal fun Kind.colour(): Color = when (this) {
    Kind.Synth, Kind.Fx, Kind.Sample -> SonicPiColors.Pink
    Kind.Note, Kind.Chord, Kind.Scale -> SonicPiColors.Blue
    Kind.Opt, Kind.OptVal -> SonicPiColors.Yellow
    else -> SonicPiColors.Green
}

@Composable
private fun KindBadge(kind: Kind) {
    val c = kind.colour()
    Box(
        Modifier.size(22.dp).background(c.copy(alpha = 0.22f), RoundedCornerShape(5.dp)),
        contentAlignment = Alignment.Center,
    ) {
        Text(kind.icon, color = c, fontSize = 13.sp, fontWeight = FontWeight.Bold)
    }
}

/**
 * Sonic Pi's completion list, docked over the keyboard's bar: each row its
 * kind, the name as it goes in, what it is. A tap takes it; the ⓘ opens its
 * doc. The best match is first, as the desktop's list has it selected.
 */
@Composable
fun SuggestionList(
    suggestions: Suggestions,
    onAccept: (Suggestion) -> Unit,
    onInfo: (Suggestion) -> Unit,
    modifier: Modifier = Modifier,
) {
    val state = rememberLazyListState()
    LaunchedEffect(suggestions.from, suggestions.items.firstOrNull()) { state.scrollToItem(0) }
    Surface(color = SonicPiColors.Raised, modifier = modifier.fillMaxWidth()) {
        LazyColumn(state = state, modifier = Modifier.heightIn(max = 184.dp).semantics { contentDescription = "Suggestions" }) {
            itemsIndexed(suggestions.items, key = { i, it -> "$i ${it.text}" }) { i, item ->
                val first = i == 0
                Row(
                    Modifier
                        .fillMaxWidth()
                        .background(if (first) SonicPiColors.Pink.copy(alpha = 0.18f) else Color.Transparent)
                        .clickable { onAccept(item) }
                        .padding(start = 10.dp, end = 2.dp)
                        .heightIn(min = 44.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    KindBadge(item.kind)
                    Spacer(Modifier.width(10.dp))
                    Text(
                        item.text, fontFamily = LocalCodeFont.current, fontSize = 15.sp, maxLines = 1,
                        color = if (item.kind in setOf(Kind.Synth, Kind.Fx, Kind.Sample)) SonicPiColors.Pink else SonicPiColors.Text,
                    )
                    Spacer(Modifier.width(12.dp))
                    Text(
                        item.summary, style = MaterialTheme.typography.bodySmall, color = SonicPiColors.Dim,
                        maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f),
                    )
                    if (item.doc.isNotEmpty()) {
                        IconButton(onClick = { onInfo(item) }, modifier = Modifier.semantics { contentDescription = "About ${item.text}" }) {
                            Text("ⓘ", color = SonicPiColors.Dim, fontSize = 18.sp)
                        }
                    }
                }
            }
        }
    }
}

/**
 * The word under the caret, as hovering over it shows on the desktop: what
 * it is in a line; a tap opens its doc.
 */
@Composable
fun WordStrip(info: WordInfo, onOpen: () -> Unit, modifier: Modifier = Modifier) {
    Row(
        modifier
            .fillMaxWidth()
            .background(SonicPiColors.Raised)
            .clickable(onClick = onOpen)
            .padding(horizontal = 10.dp, vertical = 8.dp)
            .semantics { contentDescription = "${info.title}: ${info.summary}. Open its doc" },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        KindBadge(info.kind)
        Spacer(Modifier.width(10.dp))
        Text(info.title, fontFamily = LocalCodeFont.current, fontSize = 14.sp, color = info.kind.colour(), fontWeight = FontWeight.Bold, maxLines = 1)
        Spacer(Modifier.width(10.dp))
        Text(info.summary, style = MaterialTheme.typography.bodySmall, color = SonicPiColors.Dim, maxLines = 1,
            overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
        Text("ⓘ", color = SonicPiColors.Dim, fontSize = 18.sp, modifier = Modifier.padding(start = 6.dp))
    }
}

/** A word's whole doc, as the desktop's completion detail pane shows it: its name, what it is, how it is used, the doc. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DocSheet(info: WordInfo, functions: Set<String>, onDismiss: () -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, containerColor = SonicPiColors.Editor) {
        Column(
            Modifier.verticalScroll(rememberScrollState()).padding(horizontal = 20.dp).padding(bottom = 24.dp).navigationBarsPadding(),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                KindBadge(info.kind)
                Spacer(Modifier.width(10.dp))
                Text(info.title, style = MaterialTheme.typography.headlineSmall, fontFamily = LocalCodeFont.current, color = SonicPiColors.Pink)
            }
            if (info.summary.isNotEmpty() && info.summary != info.title) {
                Text(info.summary, style = MaterialTheme.typography.titleMedium, color = SonicPiColors.Text)
            }
            if (info.usage.isNotEmpty()) {
                Text(
                    remember(info.usage, functions) { Highlighter.highlight(info.usage, functions) },
                    fontFamily = LocalCodeFont.current, fontSize = 14.sp, color = SonicPiColors.Text,
                    modifier = Modifier.fillMaxWidth().background(SonicPiColors.Black, RoundedCornerShape(6.dp)).padding(12.dp),
                )
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
            Text(
                remember(info.doc) { AnnotatedString.fromHtml(info.doc.replace(Regex("<table[\\s\\S]*?</table>"), "")) },
                style = MaterialTheme.typography.bodyLarge, color = SonicPiColors.Text,
            )
        }
    }
}

/** A suggestion's doc, as a [WordInfo] for the sheet. */
fun Suggestion.info() = WordInfo(text, kind, summary, usage, doc)
