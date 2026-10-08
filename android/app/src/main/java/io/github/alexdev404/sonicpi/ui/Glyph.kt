// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.ui

import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource

/**
 * One of the web version's toolbar glyphs (app/web/web/data/toolbar: ▶ ■ + ⌐ Δ),
 * a mask tinted [tint] as its stylesheet tints it; [fallback] if it is missing.
 */
@Composable
fun Glyph(name: String, tint: Color, fallback: Int, modifier: Modifier = Modifier) {
    val assets = LocalContext.current.assets
    val bitmap = remember(assets, name) {
        runCatching { assets.open("sonicpi/gui/glyphs/$name.png").use { BitmapFactory.decodeStream(it)?.asImageBitmap() } }.getOrNull()
    }
    if (bitmap != null) Image(bitmap, contentDescription = null, colorFilter = ColorFilter.tint(tint), modifier = modifier)
    else Icon(painterResource(fallback), contentDescription = null, tint = tint, modifier = modifier)
}
