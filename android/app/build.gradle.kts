// SPDX-License-Identifier: AGPL-3.0-or-later
import java.security.MessageDigest

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
    id("io.github.takahirom.roborazzi")
}

// The Sonic Pi checkout the app is built from (the submodule beside android/).
val sonicPi: File = rootDir.resolve("../sonic-pi").canonicalFile
val nativeOut = layout.buildDirectory.dir("sonicpi-native")
val abis = listOf("arm64-v8a", "x86_64")

android {
    namespace = "io.github.alexdev404.sonicpi"
    compileSdk = 37      // what the newest Compose is built against; the app targets Android 16 (below)
    ndkVersion = "29.0.14206865"

    defaultConfig {
        applicationId = "io.github.alexdev404.sonicpi"
        minSdk = 30          // Android 11: what the native core's libc calls need
        targetSdk = 36       // Android 16
        versionCode = 1
        versionName = "0.1.0"
        ndk { abiFilters += abis }
        externalNativeBuild {
            cmake {
                arguments += listOf("-DSP_BUILD=${nativeOut.get().asFile}", "-DANDROID_STL=c++_static")
                targets += "sonicpi"   // the app's library alone, not the engine tree's own programs
            }
        }
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.31.6"
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            // Signed with the debug key so a CI build installs as it is; a store
            // release signs with its own key.
            signingConfig = signingConfigs.getByName("debug")
        }
    }

    buildFeatures { compose = true }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    testOptions {
        unitTests.isIncludeAndroidResources = true
        unitTests.all {
            // Robolectric's Android 16 image reaches into the JDK's file descriptors.
            it.jvmArgs("--add-opens=java.base/java.io=ALL-UNNAMED", "--add-exports=java.base/jdk.internal.access=ALL-UNNAMED",
                "--add-opens=java.base/jdk.internal.access=ALL-UNNAMED")
        }
    }

    packaging {
        // The samples are FLAC already: storing them uncompressed costs nothing and installs faster.
        resources.excludes += "/META-INF/{AL2.0,LGPL2.1}"
    }
    androidResources {
        noCompress += listOf("flac", "scsyndef", "wav", "dat")
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

// ── Sonic Pi's own files, from the checkout, into the APK's assets ──────────
//
// Under assets/sonicpi/: the synthdefs, samples, random tables and piano
// table the engine reads (extracted to the app's private storage on first
// launch, since the engine opens files by path), the example programs, the
// language reference for the Learn screen, and the Hack font Sonic Pi's
// editor uses. manifest.txt lists every file with its size, and its hash is
// the version the app compares to decide whether to extract again.
abstract class PrepareSonicPiAssets : DefaultTask() {
    @get:Internal abstract val sonicPiRoot: Property<File>
    @get:InputFiles @get:PathSensitive(PathSensitivity.RELATIVE) abstract val sources: ConfigurableFileCollection
    @get:OutputDirectory abstract val outputDir: DirectoryProperty

    @TaskAction
    fun prepare() {
        val root = sonicPiRoot.get()
        val out = outputDir.get().asFile.resolve("sonicpi")
        out.deleteRecursively()
        fun copyAll(from: String, to: String, keep: (File) -> Boolean) {
            val dir = root.resolve(from)
            require(dir.isDirectory) { "missing $dir: initialise the sonic-pi submodule" }
            dir.walkTopDown().filter { it.isFile && keep(it) }.forEach {
                it.copyTo(out.resolve(to).resolve(it.relativeTo(dir).path), overwrite = true)
            }
        }
        fun copyOne(from: String, to: String) {
            root.resolve(from).copyTo(out.resolve(to), overwrite = true)
        }
        copyAll("etc/synthdefs/compiled", "synthdefs") { it.extension == "scsyndef" }
        copyAll("etc/samples", "samples") { it.extension == "flac" || it.extension == "wav" }
        copyAll("etc/buffers", "buffers") { it.name.startsWith("rand-stream") && it.extension == "wav" }
        copyOne("app/external/piano/piano_wavetable.dat", "piano_wavetable.dat")
        copyAll("etc/examples", "examples") { it.extension == "rb" }
        for (ref in listOf("lang", "synths", "fx", "samples")) copyOne("app/web/web/data/reference/$ref.json", "reference/$ref.json")
        copyOne("app/gui/fonts/Hack-Regular.ttf", "fonts/Hack-Regular.ttf")
        copyOne("app/gui/fonts/Hack-Bold.ttf", "fonts/Hack-Bold.ttf")

        val lines = out.walkTopDown().filter { it.isFile }.map { "${it.relativeTo(out).invariantSeparatorsPath}\t${it.length()}" }.sorted().toList()
        val digest = MessageDigest.getInstance("SHA-256").digest(lines.joinToString("\n").toByteArray())
        out.resolve("manifest.txt").writeText(
            "# ${digest.joinToString("") { "%02x".format(it) }}\n" + lines.joinToString("\n") + "\n")
    }
}

val prepareSonicPiAssets = tasks.register<PrepareSonicPiAssets>("prepareSonicPiAssets") {
    sonicPiRoot.set(sonicPi)
    sources.from(
        sonicPi.resolve("etc/synthdefs/compiled"), sonicPi.resolve("etc/samples"), sonicPi.resolve("etc/buffers"),
        sonicPi.resolve("etc/examples"), sonicPi.resolve("app/web/web/data/reference"),
        sonicPi.resolve("app/external/piano/piano_wavetable.dat"), sonicPi.resolve("app/gui/fonts"))
    outputDir.set(layout.buildDirectory.dir("generated/sonicpi-assets"))
}

androidComponents {
    onVariants { variant ->
        variant.sources.assets?.addGeneratedSourceDirectory(prepareSonicPiAssets, PrepareSonicPiAssets::outputDir)
    }
}

// ── mruby and the runtime's bytecode, before CMake configures ───────────────
val buildMruby = tasks.register<Exec>("buildMruby") {
    description = "Builds mruby for each ABI and compiles Sonic Pi's runtime to bytecode (native/build-mruby.sh)"
    val script = rootDir.resolve("../native/build-mruby.sh")
    inputs.file(script)
    inputs.file(rootDir.resolve("../native/mruby_config.rb"))
    inputs.dir(sonicPi.resolve("app/web/runtime/lib"))
    inputs.dir(sonicPi.resolve("app/web/runtime/data"))
    outputs.dir(nativeOut)
    commandLine(script.path, nativeOut.get().asFile.path, abis.joinToString(","))
    val ndk = androidComponents.sdkComponents.ndkDirectory
    doFirst { environment("ANDROID_NDK_HOME", ndk.get().asFile.path) }
}
tasks.configureEach {
    if (name.startsWith("configureCMake") || name.startsWith("buildCMake")) dependsOn(buildMruby)
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2026.09.00")
    implementation(composeBom)
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.foundation:foundation-layout")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.core:core-ktx:1.19.1")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.11.0")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.11.0")
    implementation("androidx.lifecycle:lifecycle-process:2.11.0")
    debugImplementation("androidx.compose.ui:ui-tooling")
    debugImplementation("androidx.compose.ui:ui-test-manifest")

    testImplementation(composeBom)
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.robolectric:robolectric:4.17")
    testImplementation("androidx.compose.ui:ui-test-junit4")
    testImplementation("io.github.takahirom.roborazzi:roborazzi:1.76.0")
    testImplementation("io.github.takahirom.roborazzi:roborazzi-compose:1.76.0")
    testImplementation("io.github.takahirom.roborazzi:roborazzi-junit-rule:1.76.0")
}
