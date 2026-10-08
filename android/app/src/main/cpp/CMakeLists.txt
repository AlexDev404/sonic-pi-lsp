# SPDX-License-Identifier: AGPL-3.0-or-later
# libsonicpi.so: the native core (../../../../../native) with the app's AAudio
# output and the JNI the Kotlin side calls (engine/NativeBridge.kt).
cmake_minimum_required(VERSION 3.24)
project(SonicPiAndroid C CXX)

add_subdirectory("${CMAKE_CURRENT_SOURCE_DIR}/../../../../../native" native)

add_library(sonicpi SHARED sonicpi_jni.cpp audio_output.cpp)
target_link_libraries(sonicpi PRIVATE sonicpi_core aaudio log android)
target_compile_features(sonicpi PRIVATE cxx_std_20)
# 16 KB pages: what Android 15+ devices may use, and what Play requires.
target_link_options(sonicpi PRIVATE "-Wl,-z,max-page-size=16384")
