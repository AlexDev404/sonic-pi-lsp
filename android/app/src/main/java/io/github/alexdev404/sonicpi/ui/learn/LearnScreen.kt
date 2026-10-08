// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.learn

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.input.TextFieldLineLimits
import androidx.compose.foundation.text.input.rememberTextFieldState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.fromHtml
import androidx.compose.ui.text.style.TextDecoration
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
 * Sonic Pi's help panel: the example programs and the reference for the
 * language, synths, FX and samples, as the desktop lays it out. A row of
 * pill tabs, a filter, the list (the chosen row in pink) and the page beside
 * it, or, on a phone, the list and then the page. Code opens into the
 * current buffer; a sample plays at a tap.
 */
@Composable
fun LearnScreen(
    sections: List<LibrarySection>,
    functions: Set<String>,
    onOpenCode: (String) -> Unit,
    onPlay: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val p = SonicPiColors
    var tab by rememberSaveable { mutableIntStateOf(0) }
    var openKey by rememberSaveable { mutableStateOf<String?>(null) }
    val section = sections.getOrNull(tab)
    val open = section?.items?.firstOrNull { it.key == openKey }
    val wide = LocalConfiguration.current.screenWidthDp >= 720

    BackHandler(enabled = !wide && open != null) { openKey = null }

    Column(modifier.fillMaxSize().background(p.Background)) {
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 8.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            sections.forEachIndexed { i, s -> Pill(s.title, selected = i == tab) { tab = i; openKey = null } }
        }
        if (section == null) return@Column
        if (wide) {
            Row(Modifier.fillMaxSize()) {
                ItemList(section, open?.key ?: section.items.firstOrNull()?.key, onOpen = { openKey = it.key }, modifier = Modifier.width(280.dp).fillMaxHeight())
                Box(Modifier.width(1.dp).fillMaxHeight().background(p.Border))
                (open ?: section.items.firstOrNull())?.let { Page(it, functions, onOpenCode, onPlay, Modifier.weight(1f)) }
            }
        } else if (open != null) {
            Column {
                Row(
                    Modifier.fillMaxWidth().clickable { openKey = null }.padding(horizontal = 8.dp, vertical = 4.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(painterResource(R.drawable.ic_back), contentDescription = null, tint = p.Pink, modifier = Modifier.size(20.dp))
                    Spacer(Modifier.width(6.dp))
                    Text(section.title, color = p.Pink, fontSize = 14.sp)
                }
                Page(open, functions, onOpenCode, onPlay, Modifier.weight(1f))
            }
        } else {
            ItemList(section, null, onOpen = { openKey = it.key }, modifier = Modifier.fillMaxSize())
        }
    }
}

/** A help tab, as the desktop's pill row has them: pink and filled when chosen, outlined otherwise. */
@Composable
private fun Pill(label: String, selected: Boolean, onClick: () -> Unit) {
    val p = SonicPiColors
    val shape = RoundedCornerShape(6.dp)
    Box(
        Modifier
            .height(34.dp)
            .clip(shape)
            .background(if (selected) p.Pink else p.Background)
            .border(1.5.dp, if (selected) p.Pink else p.Button.copy(alpha = 0.6f), shape)
            .clickable(onClick = onClick)
            .padding(horizontal = 12.dp)
            .semantics { contentDescription = label + if (selected) ", selected" else "" },
        contentAlignment = Alignment.Center,
    ) {
        Text(label, fontSize = 15.sp, fontWeight = FontWeight.SemiBold, color = if (selected) p.OnButton else p.Foreground)
    }
}

@Composable
private fun ItemList(section: LibrarySection, selected: String?, onOpen: (LibraryItem) -> Unit, modifier: Modifier) {
    val p = SonicPiColors
    val search = rememberTextFieldState()
    val query = search.text.toString().trim().lowercase()
    val items = remember(section, query) {
        if (query.isEmpty()) section.items
        else section.items.filter { query in it.title.lowercase() || query in it.summary.lowercase() || it.samples.any { s -> query in s } }
    }
    Column(modifier) {
        // The filter: a magnifier and the words, on a hairline, as the desktop's.
        Row(Modifier.padding(horizontal = 10.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(painterResource(R.drawable.ic_search), contentDescription = null, tint = p.Muted, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Box(Modifier.weight(1f)) {
                if (search.text.isEmpty()) Text("Filter ${section.title}…", color = p.Muted, fontSize = 15.sp)
                BasicTextField(
                    state = search,
                    lineLimits = TextFieldLineLimits.SingleLine,
                    textStyle = TextStyle(color = p.Foreground, fontSize = 15.sp),
                    cursorBrush = SolidColor(p.Pink),
                    modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Filter ${section.title}" },
                )
            }
        }
        LazyColumn(Modifier.fillMaxSize().padding(horizontal = 6.dp)) {
            var lastGroup = ""
            for (item in items) {
                // Examples come in levels, as the desktop's chapters.
                if (section.title == "Examples" && item.summary != lastGroup) {
                    lastGroup = item.summary
                    item(key = "group ${item.summary}") {
                        Text(item.summary, fontSize = 15.sp, fontWeight = FontWeight.Bold, color = p.Foreground,
                            modifier = Modifier.padding(start = 10.dp, top = 10.dp, bottom = 4.dp))
                    }
                }
                item(key = item.key) {
                    val chosen = item.key == selected
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(3.dp))
                            .background(if (chosen) p.Pink else p.Background)
                            .clickable { onOpen(item) }
                            .padding(horizontal = 10.dp, vertical = 7.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(item.title, fontSize = 15.sp, color = if (chosen) p.OnButton else p.Foreground, maxLines = 1,
                            overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                        if (section.title != "Examples" && item.summary.isNotEmpty() && item.summary != item.title) {
                            Spacer(Modifier.width(10.dp))
                            Text(item.summary, fontSize = 13.sp, color = if (chosen) p.OnButton.copy(alpha = 0.8f) else p.Muted,
                                maxLines = 1, overflow = TextOverflow.Ellipsis)
                        }
                    }
                }
            }
        }
    }
}

/** A help page: a pink title over a pink rule, what it is, its usage, the doc, its opts and examples. */
@Composable
private fun Page(
    item: LibraryItem,
    functions: Set<String>,
    onOpenCode: (String) -> Unit,
    onPlay: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val p = SonicPiColors
    val code = LocalCodeFont.current
    Column(
        modifier.fillMaxHeight().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 24.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Column {
            Text(item.title, fontSize = 24.sp, fontWeight = FontWeight.Bold, color = p.Pink, modifier = Modifier.padding(top = 8.dp, bottom = 6.dp))
            Box(Modifier.fillMaxWidth().height(1.dp).background(p.Pink))
        }
        if (item.summary.isNotEmpty() && item.summary != item.title) {
            Text(item.summary, fontFamily = code, fontSize = 12.sp, color = p.Foreground)
        }
        if (item.usage.isNotEmpty()) CodeBox(item.usage, functions, onOpen = { onOpenCode(item.usage) })
        if (item.docHtml.isNotEmpty()) {
            Text(
                remember(item.docHtml, p) {
                    AnnotatedString.fromHtml(
                        item.docHtml,
                        TextLinkStyles(SpanStyle(color = p.Pink, textDecoration = TextDecoration.Underline)),
                    )
                },
                fontSize = 15.sp, lineHeight = 21.sp, color = p.Foreground,
            )
        }
        if (item.samples.isNotEmpty()) {
            Heading("Samples")
            for (s in item.samples) {
                Row(
                    Modifier.fillMaxWidth().clip(RoundedCornerShape(4.dp)).border(1.dp, p.Button.copy(alpha = 0.35f), RoundedCornerShape(4.dp))
                        .clickable { onPlay("sample :$s") }.padding(horizontal = 10.dp, vertical = 8.dp)
                        .semantics { contentDescription = "Play :$s" },
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(painterResource(R.drawable.ic_play), contentDescription = null, tint = p.Pink, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(10.dp))
                    Text(remember(s, p) { Highlighter.highlight("sample :$s", functions, p) }, fontFamily = code, fontSize = 13.sp, color = p.Foreground)
                }
            }
        }
        val opts = item.opts.filterNot { it.first.endsWith("_slide") || it.first.endsWith("_slide_shape") || it.first.endsWith("_slide_curve") }
        if (opts.isNotEmpty()) {
            Heading("Opts")
            for ((name, default, doc) in opts) {
                Column {
                    Row(verticalAlignment = Alignment.Bottom) {
                        Text("$name:", fontFamily = code, fontSize = 13.sp, color = p.Blue, textDecoration = TextDecoration.Underline)
                        Spacer(Modifier.width(8.dp))
                        Text(default, fontFamily = code, fontSize = 13.sp, color = p.Foreground)
                    }
                    if (doc.isNotEmpty()) Text(doc, fontSize = 14.sp, color = p.Muted)
                }
            }
        }
        if (item.code.isNotEmpty()) {
            if (item.usage.isNotEmpty() || item.docHtml.isNotEmpty()) Heading("Examples")
            for ((i, c) in item.code.withIndex()) {
                if (item.code.size > 1) Text("Example ${i + 1}", fontFamily = code, fontSize = 12.sp, color = p.Foreground)
                CodeBox(c, functions, onOpen = { onOpenCode(c) }, onPlay = { onPlay(c) })
            }
        }
    }
}

@Composable
private fun Heading(text: String) {
    Text(text, fontSize = 20.sp, fontWeight = FontWeight.Bold, color = SonicPiColors.Blue, modifier = Modifier.padding(top = 8.dp))
}

/** Code on a help page, in a bordered box as the desktop's, with its actions at the top right. */
@Composable
private fun CodeBox(code: String, functions: Set<String>, onOpen: () -> Unit, onPlay: (() -> Unit)? = null) {
    val p = SonicPiColors
    val shape = RoundedCornerShape(4.dp)
    Box(Modifier.fillMaxWidth().clip(shape).background(p.Base.copy(alpha = if (p.dark) 0.35f else 0.4f)).border(1.dp, p.Button.copy(alpha = 0.35f), shape)) {
        Text(
            remember(code, functions, p) { Highlighter.highlight(code.trimEnd(), functions, p) },
            fontFamily = LocalCodeFont.current, fontSize = 13.sp, lineHeight = 19.sp, color = p.Foreground, softWrap = false,
            modifier = Modifier.horizontalScroll(rememberScrollState()).padding(start = 12.dp, end = 76.dp, top = 10.dp, bottom = 10.dp),
        )
        Row(Modifier.align(Alignment.TopEnd).padding(4.dp)) {
            if (onPlay != null) BoxAction(R.drawable.ic_play, "Play this", onPlay)
            BoxAction(R.drawable.ic_copy, "Open in the buffer", onOpen)
        }
    }
}

@Composable
private fun BoxAction(icon: Int, description: String, onClick: () -> Unit) {
    Box(
        Modifier.size(34.dp).clip(RoundedCornerShape(4.dp)).clickable(onClick = onClick).semantics { contentDescription = description },
        contentAlignment = Alignment.Center,
    ) {
        Icon(painterResource(icon), contentDescription = null, tint = SonicPiColors.Muted, modifier = Modifier.size(18.dp))
    }
}
