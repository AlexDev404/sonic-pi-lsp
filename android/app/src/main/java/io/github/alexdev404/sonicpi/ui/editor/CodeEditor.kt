// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.editor

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.LocalContentColor
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.alexdev404.sonicpi.R
import io.github.alexdev404.sonicpi.ui.theme.LocalCodeFont
import io.github.alexdev404.sonicpi.ui.theme.SonicPiColors

/**
 * The code editor, as Sonic Pi's: its colours, its font, line numbers in
 * italics in the margin (wrapped lines keep theirs), the caret's line
 * marked, the line an error came from marked, and a new line indented as
 * the one before it.
 */
@Composable
fun CodeEditor(
    value: TextFieldValue,
    onValueChange: (TextFieldValue) -> Unit,
    functions: Set<String>,
    modifier: Modifier = Modifier,
    fontSize: TextUnit = 14.sp,
    errorLine: Int = 0,
) {
    val p = SonicPiColors
    val font = LocalCodeFont.current
    val style = TextStyle(fontFamily = font, fontSize = fontSize, lineHeight = fontSize * 1.45f, color = p.Foreground)
    val measurer = rememberTextMeasurer()
    var layout by remember { mutableStateOf<TextLayoutResult?>(null) }
    val transformation = remember(functions, p) { HighlightTransformation(functions, p) }
    val density = LocalDensity.current
    val gutterWidth = with(density) { (fontSize.toPx() * 2.6f).toDp() } + 12.dp
    val scroll = rememberScrollState()
    val caretLine = value.text.lineNumberAt(value.selection.start)

    Box(modifier.background(p.Background).verticalScroll(scroll)) {
        Row(
            Modifier.fillMaxWidth().padding(vertical = 8.dp).drawBehind {
                // The caret's line, the width of the editor (CaretLineBackground).
                val l = layout ?: return@drawBehind
                if (p.CaretLine == p.Background || !value.selection.collapsed) return@drawBehind
                val row = l.getLineForOffset(value.selection.start.coerceAtMost(value.text.length))
                drawRect(p.CaretLine, Offset(0f, l.getLineTop(row)), Size(size.width, l.getLineBottom(row) - l.getLineTop(row)))
            },
        ) {
            // Line numbers, drawn where each logical line starts in the layout.
            Box(
                Modifier
                    .width(gutterWidth)
                    .height(with(density) { (layout?.size?.height ?: 0).toDp() })
                    .drawBehind {
                        val l = layout ?: return@drawBehind
                        val text = value.text
                        val numberStyle = style.copy(color = p.Margin, textAlign = TextAlign.End, fontStyle = FontStyle.Italic)
                        var line = 1
                        var offset = 0
                        while (true) {
                            val row = l.getLineForOffset(offset.coerceAtMost(text.length))
                            val top = l.getLineTop(row)
                            if (line == errorLine) {
                                drawRect(p.Red.copy(alpha = 0.35f), Offset(0f, top), Size(size.width, l.getLineBottom(row) - top))
                            }
                            val measured = measurer.measure(line.toString(), if (line == caretLine) numberStyle.copy(color = p.Foreground) else numberStyle)
                            drawText(measured, topLeft = Offset(size.width - measured.size.width - 8.dp.toPx(), top))
                            val next = text.indexOf('\n', offset)
                            if (next < 0) break
                            offset = next + 1
                            line++
                        }
                    }
            )
            BasicTextField(
                value = value,
                onValueChange = { onValueChange(autoIndent(value, it)) },
                textStyle = style,
                cursorBrush = SolidColor(p.Pink),
                visualTransformation = transformation,
                onTextLayout = { layout = it },
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(start = 6.dp, end = 12.dp)
                    .semantics { contentDescription = "Code editor" },
            )
        }
    }
}

/** The 1-based line [offset] is on. */
internal fun String.lineNumberAt(offset: Int): Int {
    var n = 1
    for (i in 0 until offset.coerceAtMost(length)) if (this[i] == '\n') n++
    return n
}

/**
 * Under the editor, as on the desktop: where the caret is
 * ("Line: 3,  Position: 5"), and the buffers as a strip of |0| |1| … |9|,
 * the one being edited in pink.
 */
@Composable
fun BufferStrip(
    count: Int,
    current: Int,
    caret: Pair<Int, Int>,
    onSelect: (Int) -> Unit,
    modifier: Modifier = Modifier,
) {
    val p = SonicPiColors
    val code = LocalCodeFont.current
    Column(modifier.fillMaxWidth().background(p.Background).padding(horizontal = 8.dp, vertical = 4.dp)) {
        Text(
            "Line: ${caret.first},  Position: ${caret.second}",
            fontFamily = code, fontSize = 12.sp, color = p.Foreground,
            modifier = Modifier.padding(start = 4.dp, bottom = 4.dp),
        )
        Row(Modifier.fillMaxWidth().height(30.dp).background(p.Button)) {
            for (i in 0 until count) {
                val selected = i == current
                Box(
                    Modifier
                        .weight(1f)
                        .fillMaxHeight()
                        .background(if (selected) p.Pink else p.Button)
                        .clickable { onSelect(i) }
                        .semantics { contentDescription = "Buffer $i" + if (selected) ", selected" else "" },
                    contentAlignment = Alignment.Center,
                ) {
                    Text("|$i|", fontFamily = code, fontSize = 15.sp, color = p.OnButton, maxLines = 1)
                }
            }
        }
    }
}

/** A newline typed on its own takes the indentation of the line it ended (and a step more after `do`). */
internal fun autoIndent(old: TextFieldValue, new: TextFieldValue): TextFieldValue {
    val o = old.text
    val n = new.text
    if (n.length != o.length + 1 || !new.selection.collapsed) return new
    val at = new.selection.start - 1
    if (at < 0 || n[at] != '\n' || n.removeRange(at, at + 1) != o) return new
    val lineStart = n.lastIndexOf('\n', at - 1) + 1
    val indent = indentAfter(n.substring(lineStart, at))
    if (indent.isEmpty()) return new
    val text = n.substring(0, at + 1) + indent + n.substring(at + 1)
    return new.copy(text = text, selection = TextRange(at + 1 + indent.length))
}

/** Inserts `text` at the selection (replacing it), the cursor after it. */
fun TextFieldValue.insert(text: String): TextFieldValue {
    val start = selection.min
    val end = selection.max
    val newText = this.text.substring(0, start) + text + this.text.substring(end)
    return copy(text = newText, selection = TextRange(start + text.length))
}

/**
 * The keys a phone's keyboard hides behind a second page, and the words code
 * is made of: one tap each, above the keyboard.
 */
@Composable
fun KeyBar(
    onInsert: (String) -> Unit,
    onUndo: () -> Unit,
    onRedo: () -> Unit,
    canUndo: Boolean,
    canRedo: Boolean,
    modifier: Modifier = Modifier,
) {
    val keys = listOf(":", ",", ".", "(", ")", "[", "]", "|", "\"", "#", "=", "_", "do", "end", "{", "}")
    val p = SonicPiColors
    Row(
        modifier
            .fillMaxWidth()
            .background(p.Base)
            .horizontalScroll(rememberScrollState())
            .padding(horizontal = 6.dp, vertical = 5.dp),
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        KeyChip(onClick = onUndo, enabled = canUndo, description = "Undo") {
            Icon(painterResource(R.drawable.ic_undo), null, Modifier.size(18.dp))
        }
        KeyChip(onClick = onRedo, enabled = canRedo, description = "Redo") {
            Icon(painterResource(R.drawable.ic_redo), null, Modifier.size(18.dp))
        }
        KeyChip(onClick = { onInsert("  ") }, description = "Indent") {
            Icon(painterResource(R.drawable.ic_tab), null, Modifier.size(18.dp))
        }
        for (k in keys) {
            KeyChip(onClick = { onInsert(k) }, description = k) {
                Text(k, fontFamily = LocalCodeFont.current, fontSize = 16.sp)
            }
        }
    }
}

/** A key as the desktop's toolbar draws a button: flat, grey, white on it. */
@Composable
private fun KeyChip(onClick: () -> Unit, description: String, enabled: Boolean = true, content: @Composable () -> Unit) {
    val p = SonicPiColors
    Box(
        Modifier
            .height(38.dp)
            .defaultMinSize(minWidth = 38.dp)
            .clip(RoundedCornerShape(2.dp))
            .background(p.Button)
            .clickable(enabled = enabled, onClick = onClick)
            .padding(PaddingValues(horizontal = 10.dp))
            .semantics { contentDescription = description },
        contentAlignment = Alignment.Center,
    ) {
        CompositionLocalProvider(LocalContentColor provides if (enabled) p.OnButton else p.OnButton.copy(alpha = 0.35f)) {
            content()
        }
    }
}
