// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.learn

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.input.rememberTextFieldState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AssistChip
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.PrimaryScrollableTabRow
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.fromHtml
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.alexdev404.sonicpi.R
import io.github.alexdev404.sonicpi.data.LibraryItem
import io.github.alexdev404.sonicpi.data.LibrarySection
import io.github.alexdev404.sonicpi.ui.editor.Highlighter
import io.github.alexdev404.sonicpi.ui.theme.LocalCodeFont
import io.github.alexdev404.sonicpi.ui.theme.SonicPiColors

/**
 * Learn: the example programs, and Sonic Pi's reference for its functions,
 * synths, FX and samples. Code opens into the current buffer; a sample can
 * be heard with a tap.
 */
@Composable
fun LearnScreen(
    sections: List<LibrarySection>,
    functions: Set<String>,
    onOpenCode: (String) -> Unit,
    onPlay: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    var tab by rememberSaveable { mutableIntStateOf(0) }
    var openKey by rememberSaveable { mutableStateOf<String?>(null) }
    val section = sections.getOrNull(tab)
    val open = section?.items?.firstOrNull { it.key == openKey }

    BackHandler(enabled = open != null) { openKey = null }

    Column(modifier.fillMaxSize()) {
        PrimaryScrollableTabRow(selectedTabIndex = tab, edgePadding = 8.dp, containerColor = MaterialTheme.colorScheme.surface) {
            sections.forEachIndexed { i, s ->
                Tab(
                    selected = i == tab, onClick = { tab = i; openKey = null }, text = { Text(s.title) },
                    selectedContentColor = SonicPiColors.Pink, unselectedContentColor = SonicPiColors.Dim,
                )
            }
        }
        if (open != null && section != null) {
            Detail(open, section.title, functions, onBack = { openKey = null }, onOpenCode = onOpenCode, onPlay = onPlay)
        } else if (section != null) {
            ItemList(section, onOpen = { openKey = it.key })
        }
    }
}

@Composable
private fun ItemList(section: LibrarySection, onOpen: (LibraryItem) -> Unit) {
    val search = rememberTextFieldState()
    val query = search.text.toString().trim().lowercase()
    val items = remember(section, query) {
        if (query.isEmpty()) section.items
        else section.items.filter { query in it.title.lowercase() || query in it.summary.lowercase() || it.samples.any { s -> query in s } }
    }
    LazyColumn(Modifier.fillMaxSize()) {
        item {
            OutlinedTextField(
                state = search,
                placeholder = { Text("Search ${section.title.lowercase()}") },
                lineLimits = androidx.compose.foundation.text.input.TextFieldLineLimits.SingleLine,
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
            )
        }
        items(items, key = { it.key }) { item ->
            Column(
                Modifier.fillMaxWidth().clickable { onOpen(item) }.padding(horizontal = 16.dp, vertical = 12.dp)
            ) {
                Text(item.title, style = MaterialTheme.typography.titleMedium, color = SonicPiColors.Pink)
                if (item.summary.isNotEmpty()) {
                    Text(item.summary, style = MaterialTheme.typography.bodyMedium, color = SonicPiColors.Dim, maxLines = 2, overflow = TextOverflow.Ellipsis)
                }
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
        }
    }
}

@Composable
private fun Detail(
    item: LibraryItem,
    sectionTitle: String,
    functions: Set<String>,
    onBack: () -> Unit,
    onOpenCode: (String) -> Unit,
    onPlay: (String) -> Unit,
) {
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(bottom = 24.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(start = 4.dp, end = 16.dp, top = 8.dp)) {
            IconButton(onClick = onBack) { Icon(painterResource(R.drawable.ic_back), contentDescription = "Back to $sectionTitle") }
            Column {
                Text(item.title, style = MaterialTheme.typography.headlineSmall, color = SonicPiColors.Pink)
                if (item.summary.isNotEmpty()) Text(item.summary, style = MaterialTheme.typography.bodyMedium, color = SonicPiColors.Dim)
            }
        }
        Column(Modifier.padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (item.usage.isNotEmpty()) CodeBlock(item.usage, functions, action = "Open in buffer" to { onOpenCode(item.usage) })
            if (item.docHtml.isNotEmpty()) {
                Text(
                    remember(item.docHtml) { AnnotatedString.fromHtml(item.docHtml) },
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.onSurface,
                )
            }
            if (item.samples.isNotEmpty()) {
                Text("Tap a sample to hear it", style = MaterialTheme.typography.labelLarge, color = SonicPiColors.Dim)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    for (s in item.samples) AssistChip(onClick = { onPlay("sample :$s") }, label = { Text(":$s", fontFamily = LocalCodeFont.current) })
                }
            }
            if (item.opts.isNotEmpty()) {
                Text("Opts", style = MaterialTheme.typography.titleMedium)
                for ((name, default, doc) in item.opts.filterNot { it.first.endsWith("_slide") || it.first.endsWith("_slide_shape") || it.first.endsWith("_slide_curve") }) {
                    Column {
                        Row(verticalAlignment = Alignment.Bottom) {
                            Text("$name:", fontFamily = LocalCodeFont.current, color = SonicPiColors.Text, fontWeight = FontWeight.Bold)
                            Spacer(Modifier.width(8.dp))
                            Text(default, fontFamily = LocalCodeFont.current, color = SonicPiColors.Blue)
                        }
                        if (doc.isNotEmpty()) Text(doc, style = MaterialTheme.typography.bodyMedium, color = SonicPiColors.Dim)
                    }
                }
            }
            for ((i, code) in item.code.withIndex()) {
                if (item.code.size > 1) Text("Example ${i + 1}", style = MaterialTheme.typography.labelLarge, color = SonicPiColors.Dim)
                CodeBlock(code, functions, action = "Open in buffer" to { onOpenCode(code) }, play = { onPlay(code) })
            }
        }
    }
}

@Composable
private fun CodeBlock(code: String, functions: Set<String>, action: Pair<String, () -> Unit>? = null, play: (() -> Unit)? = null) {
    Column(Modifier.fillMaxWidth().background(SonicPiColors.Editor, RoundedCornerShape(10.dp))) {
        Text(
            remember(code, functions) { Highlighter.highlight(code.trimEnd(), functions) },
            fontFamily = LocalCodeFont.current,
            fontSize = 13.sp,
            lineHeight = 19.sp,
            color = SonicPiColors.Text,
            softWrap = false,
            modifier = Modifier.horizontalScroll(rememberScrollState()).padding(12.dp),
        )
        if (action != null || play != null) {
            Row(Modifier.padding(start = 8.dp, end = 8.dp, bottom = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (play != null) FilledTonalButton(onClick = play) {
                    Icon(painterResource(R.drawable.ic_play), null)
                    Spacer(Modifier.width(4.dp))
                    Text("Play")
                }
                if (action != null) FilledTonalButton(onClick = action.second) {
                    Icon(painterResource(R.drawable.ic_copy), null)
                    Spacer(Modifier.width(4.dp))
                    Text(action.first)
                }
            }
        }
    }
    Spacer(Modifier.height(4.dp))
}
