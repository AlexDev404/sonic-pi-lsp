// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.theme

import android.graphics.BitmapFactory
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight

/**
 * One of Sonic Pi's colour themes, as the desktop app defines it
 * (app/gui/model/sonicpitheme.cpp: lightTheme and darkTheme), named for
 * what each colour is for.
 */
@Immutable
data class SonicPiPalette(
    val dark: Boolean,
    /** The window, the editor, the panes and the log ("Background"). */
    val Background: Color,
    /** Menus and raised surfaces ("Base"). */
    val Base: Color,
    /** Text ("Foreground"). */
    val Foreground: Color,
    /** Toolbar buttons and unselected tabs ("Button", "Tab"), with [OnButton] text. */
    val Button: Color,
    val OnButton: Color,
    /** Hairlines between panes ("WindowBorder"). */
    val Border: Color,
    /** What the help and the log say quietly: panel titles, summaries. */
    val Muted: Color,
    /** deeppink: the selected tab, the caret, symbols and Sonic Pi's functions, links. */
    val Pink: Color,
    /** Numbers ("NumberForeground"), and the log's first stream. */
    val Blue: Color,
    /** Ruby's keywords ("KeywordForeground"). */
    val Keyword: Color,
    /** Strings. */
    val Green: Color,
    val Orange: Color,
    val Red: Color,
    val Comment: Color,
    /** Line numbers ("MarginForeground"). */
    val Margin: Color,
    /** The caret's line ("CaretLineBackground"). */
    val CaretLine: Color,
    /** The status line ("StatusBarText"). */
    val Status: Color,
)

private val Deeppink = Color(0xFFFF1493)
private val Darkorange = Color(0xFFFF8C00)

val LightPalette = SonicPiPalette(
    dark = false,
    Background = Color.White, Base = Color(0xFFEDEDED), Foreground = Color(0xFF5E5E5E),
    Button = Color(0xFF5E5E5E), OnButton = Color.White, Border = Color(0xFFEDEDED), Muted = Color(0xFF8E8E8E),
    Pink = Deeppink, Blue = Color(0xFF1E90FF), Keyword = Color(0xFFFF8C00), Green = Color(0xFF61CE3C),
    Orange = Darkorange, Red = Color(0xFFFF0000), Comment = Color(0xFF5E5E5E), Margin = Color(0xFFD3D3D3),
    CaretLine = Color.White, Status = Color(0xFF5E5E5E),
)

val DarkPalette = SonicPiPalette(
    dark = true,
    Background = Color.Black, Base = Color(0xFF1E1E1E), Foreground = Color(0xFFEDEDED),
    Button = Color(0xFF5E5E5E), OnButton = Color.White, Border = Color(0xFF1E1E1E), Muted = Color(0xFF8E8E8E),
    Pink = Deeppink, Blue = Color(0xFF4C83FF), Keyword = Color(0xFFFBDE2D), Green = Color(0xFF61CE3C),
    Orange = Darkorange, Red = Color(0xFFFF0000), Comment = Color(0xFF5E5E5E), Margin = Color(0xFF5E5E5E),
    CaretLine = Color(0xFF0D0D0D), Status = Color(0xFF4C83FF),
)

val LocalPalette = staticCompositionLocalOf { DarkPalette }

/** The colours of the theme in effect. */
val SonicPiColors: SonicPiPalette
    @Composable @ReadOnlyComposable get() = LocalPalette.current

/** The editor's font: Hack, as Sonic Pi's editor uses, from the assets; monospace if it is missing. */
val LocalCodeFont = staticCompositionLocalOf<FontFamily> { FontFamily.Monospace }

/** The desktop's toolbar buttons (app/gui/images/toolbar/default), for the theme in effect; empty if missing. */
class ToolbarImages(private val load: (String) -> ImageBitmap?) {
    private val cache = HashMap<String, ImageBitmap?>()
    operator fun get(name: String): ImageBitmap? = cache.getOrPut(name) { load(name) }
}

val LocalToolbar = staticCompositionLocalOf { ToolbarImages { null } }

@Composable
fun SonicPiTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val assets = LocalContext.current.assets
    val code = remember(assets) {
        runCatching {
            FontFamily(
                Font("sonicpi/fonts/Hack-Regular.ttf", assets),
                Font("sonicpi/fonts/Hack-Bold.ttf", assets, FontWeight.Bold),
                Font("sonicpi/fonts/Hack-Italic.ttf", assets, FontWeight.Normal, FontStyle.Italic),
            )
        }.getOrDefault(FontFamily.Monospace)
    }
    val palette = if (dark) DarkPalette else LightPalette
    val toolbar = remember(assets, dark) {
        val prefix = if (dark) "dark" else "light"
        ToolbarImages { name ->
            runCatching { assets.open("sonicpi/gui/toolbar/$prefix-$name.png").use { BitmapFactory.decodeStream(it)?.asImageBitmap() } }.getOrNull()
        }
    }
    val scheme = if (dark) {
        darkColorScheme(
            primary = Deeppink, onPrimary = Color.White, secondary = Deeppink, onSecondary = Color.White,
            background = palette.Background, onBackground = palette.Foreground, surface = palette.Background, onSurface = palette.Foreground,
            surfaceVariant = palette.Base, onSurfaceVariant = palette.Muted, surfaceContainer = palette.Base,
            surfaceContainerLow = palette.Base, surfaceContainerHigh = palette.Base, surfaceContainerHighest = palette.Base,
            outline = palette.Button, outlineVariant = palette.Border, error = palette.Red,
        )
    } else {
        lightColorScheme(
            primary = Deeppink, onPrimary = Color.White, secondary = Deeppink, onSecondary = Color.White,
            background = palette.Background, onBackground = palette.Foreground, surface = palette.Background, onSurface = palette.Foreground,
            surfaceVariant = palette.Base, onSurfaceVariant = palette.Muted, surfaceContainer = palette.Base,
            surfaceContainerLow = palette.Base, surfaceContainerHigh = palette.Base, surfaceContainerHighest = palette.Base,
            outline = palette.Button, outlineVariant = palette.Border, error = palette.Red,
        )
    }
    CompositionLocalProvider(LocalCodeFont provides code, LocalPalette provides palette, LocalToolbar provides toolbar) {
        MaterialTheme(colorScheme = scheme, content = content)
    }
}
