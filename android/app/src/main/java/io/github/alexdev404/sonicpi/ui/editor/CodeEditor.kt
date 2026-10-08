// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.editor

import androidx.compose.foundation.background
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
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
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
 * The code editor: Sonic Pi's colours, its font, line numbers beside each
 * line (wrapped lines keep theirs), the line an error came from marked, and
 * a new line indented as the one before it.
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
    val font = LocalCodeFont.current
    val style = TextStyle(fontFamily = font, fontSize = fontSize, lineHeight = fontSize * 1.45f, color = SonicPiColors.Text)
    val measurer = rememberTextMeasurer()
    var layout by remember { mutableStateOf<TextLayoutResult?>(null) }
    val transformation = remember(functions) { HighlightTransformation(functions) }
    val density = LocalDensity.current
    val gutterWidth = with(density) { (fontSize.toPx() * 2.6f).toDp() } + 12.dp
    val scroll = rememberScrollState()

    Box(modifier.background(SonicPiColors.Editor).verticalScroll(scroll)) {
        Row(Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
            // Line numbers, drawn where each logical line starts in the layout.
            Box(
                Modifier
                    .width(gutterWidth)
                    .height(with(density) { (layout?.size?.height ?: 0).toDp() })
                    .drawBehind {
                        val l = layout ?: return@drawBehind
                        val text = value.text
                        val numberStyle = style.copy(color = SonicPiColors.Grey, textAlign = TextAlign.End)
                        var line = 1
                        var offset = 0
                        while (true) {
                            val row = l.getLineForOffset(offset.coerceAtMost(text.length))
                            val top = l.getLineTop(row)
                            if (line == errorLine) {
                                drawRect(SonicPiColors.Red.copy(alpha = 0.35f), Offset(0f, top), Size(size.width, l.getLineBottom(row) - top))
                            }
                            val measured = measurer.measure(line.toString(), numberStyle)
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
                cursorBrush = SolidColor(SonicPiColors.Pink),
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
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, modifier = modifier.fillMaxWidth()) {
        Row(
            Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 6.dp, vertical = 6.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
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
                    Text(k, fontFamily = LocalCodeFont.current, fontSize = 16.sp, color = SonicPiColors.Text)
                }
            }
        }
    }
}

@Composable
private fun KeyChip(onClick: () -> Unit, description: String, enabled: Boolean = true, content: @Composable () -> Unit) {
    Surface(
        onClick = onClick,
        enabled = enabled,
        shape = RoundedCornerShape(8.dp),
        color = SonicPiColors.Raised,
        contentColor = if (enabled) SonicPiColors.Text else SonicPiColors.Grey,
        modifier = Modifier.height(40.dp).clip(RoundedCornerShape(8.dp)).semantics { contentDescription = description },
    ) {
        Box(Modifier.defaultMinSize(minWidth = 40.dp).fillMaxHeight().padding(PaddingValues(horizontal = 12.dp)), contentAlignment = Alignment.Center) {
            content()
        }
    }
}
