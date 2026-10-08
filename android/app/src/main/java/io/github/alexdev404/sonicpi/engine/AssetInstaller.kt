// SPDX-License-Identifier: AGPL-3.0-or-later
package io.github.alexdev404.sonicpi.engine

import android.content.res.AssetManager
import java.io.File

/**
 * The engine reads its synthdefs, samples and tables by path, so the copies in
 * the APK (assets/sonicpi, from the build's prepareSonicPiAssets) are unpacked
 * into the app's private storage. No permission is needed for it, on any
 * Android version. It happens once per build of the assets: the manifest's
 * hash is the version, kept beside them.
 */
class AssetInstaller(private val assets: AssetManager, private val filesDir: File) {
    val root: File get() = File(filesDir, "sonicpi")

    fun install(progress: (Float) -> Unit): File {
        val manifest = assets.open("sonicpi/manifest.txt").bufferedReader().readLines()
        val version = manifest.firstOrNull { it.startsWith("#") }.orEmpty()
        val entries = manifest.filter { it.isNotBlank() && !it.startsWith("#") }.map {
            val (path, size) = it.split('\t')
            path to size.toLong()
        }
        val stamp = File(root, ".version")
        if (stamp.isFile && stamp.readText() == version) return root

        root.deleteRecursively()
        val total = entries.sumOf { it.second }.coerceAtLeast(1)
        var done = 0L
        val buffer = ByteArray(1 shl 16)
        for ((path, size) in entries) {
            val target = File(root, path)
            target.parentFile?.mkdirs()
            assets.open("sonicpi/$path").use { input ->
                target.outputStream().use { output ->
                    while (true) {
                        val n = input.read(buffer)
                        if (n < 0) break
                        output.write(buffer, 0, n)
                    }
                }
            }
            done += size
            progress(done.toFloat() / total)
        }
        stamp.writeText(version)
        return root
    }
}
