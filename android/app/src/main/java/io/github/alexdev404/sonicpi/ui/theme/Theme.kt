// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight

/** Sonic Pi's dark theme (app/gui/model/sonicpitheme.cpp, via the web app's themes.json). */
object SonicPiColors {
    val Pink = Color(0xFFFF1493)
    val Blue = Color(0xFF4C83FF)
    val Yellow = Color(0xFFFBDE2D)
    val Green = Color(0xFF61CE3C)
    val Orange = Color(0xFFFF8C00)
    val Red = Color(0xFFFF3B3B)
    val Black = Color(0xFF000000)
    val Editor = Color(0xFF1E1E1E)
    val Raised = Color(0xFF2A2A2A)
    val Grey = Color(0xFF5E5E5E)
    val Text = Color(0xFFEDEDED)
    val Dim = Color(0xFF9A9A9A)
    val CaretLine = Color(0xFF0D0D0D)
}

private val scheme = darkColorScheme(
    primary = SonicPiColors.Pink,
    onPrimary = Color.White,
    primaryContainer = Color(0xFF4A0A2C),
    onPrimaryContainer = Color(0xFFFFD9E6),
    // Selection (a chosen tab, chip or destination) is pink in Sonic Pi, as its own buttons are.
    secondary = Color(0xFFFF5CAD),
    onSecondary = Color.White,
    secondaryContainer = Color(0xFF4A0A2C),
    onSecondaryContainer = Color(0xFFFFD9E6),
    tertiary = SonicPiColors.Yellow,
    background = SonicPiColors.Black,
    onBackground = SonicPiColors.Text,
    surface = SonicPiColors.Black,
    onSurface = SonicPiColors.Text,
    surfaceVariant = SonicPiColors.Editor,
    onSurfaceVariant = SonicPiColors.Dim,
    surfaceContainerLowest = SonicPiColors.Black,
    surfaceContainerLow = Color(0xFF111111),
    surfaceContainer = Color(0xFF161616),
    surfaceContainerHigh = SonicPiColors.Editor,
    surfaceContainerHighest = SonicPiColors.Raised,
    outline = SonicPiColors.Grey,
    outlineVariant = Color(0xFF333333),
    error = SonicPiColors.Red,
    onError = Color.White,
    errorContainer = Color(0xFF3D0B0B),
    onErrorContainer = Color(0xFFFFDAD6),
)

/** The editor's font: Hack, as Sonic Pi's editor uses, from the assets; monospace if it is missing. */
val LocalCodeFont = staticCompositionLocalOf<FontFamily> { FontFamily.Monospace }

@Composable
fun SonicPiTheme(content: @Composable () -> Unit) {
    val assets = LocalContext.current.assets
    val code = remember(assets) {
        runCatching {
            FontFamily(
                Font("sonicpi/fonts/Hack-Regular.ttf", assets),
                Font("sonicpi/fonts/Hack-Bold.ttf", assets, FontWeight.Bold),
            )
        }.getOrDefault(FontFamily.Monospace)
    }
    androidx.compose.runtime.CompositionLocalProvider(LocalCodeFont provides code) {
        MaterialTheme(colorScheme = scheme, content = content)
    }
}
