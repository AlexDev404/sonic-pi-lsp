# SPDX-License-Identifier: AGPL-3.0-or-later
# The native core: Sonic Pi's language runtime (mruby) and its engine
# (SuperSonic: scsynth on clockwork), linked into one library with the host
# between them (src/sonic_pi_core.cpp). Built for this machine (with a test
# that renders programs to audio) and, under the NDK, for the Android app.
#
#   native/build-mruby.sh build/core [abi,...]
#   cmake -B build/native -S native -DSP_BUILD=build/core
#   cmake --build build/native && build/native/sonic_pi_render_test
cmake_minimum_required(VERSION 3.24)
project(SonicPiCore C CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

set(SP_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/../sonic-pi" CACHE PATH "The Sonic Pi checkout (the submodule)")
set(SP_BUILD "" CACHE PATH "Where native/build-mruby.sh put mruby and the runtime's bytecode")
if(NOT SP_BUILD)
    message(FATAL_ERROR "Set SP_BUILD to native/build-mruby.sh's output directory")
endif()
# Which of build-mruby.sh's mruby builds this target links: host, or android-<abi>.
if(ANDROID)
    set(SP_MRUBY_TARGET "android-${ANDROID_ABI}" CACHE STRING "")
else()
    set(SP_MRUBY_TARGET "host" CACHE STRING "")
endif()

# ── The engine ────────────────────────────────────────────────────────────
# clockwork without its device layer (the host renders), and without the
# optional Rust subsystems (MIDI, gamepad, network OSC, the timed store:
# scsynth schedules its own bundles), Link or plugin hosting. Its core crate
# (heap, sinks, ports, timelines) is Rust too, which cargo builds.
foreach(opt CLOCKWORK_DEVICE CLOCKWORK_MIDI CLOCKWORK_GAMEPAD CLOCKWORK_OSC CLOCKWORK_SCHEDULER
            CLOCKWORK_LINK CLOCKWORK_LINK_AUDIO CLOCKWORK_PLUGINS CLOCKWORK_STANDALONE BUILD_TESTS)
    set(${opt} OFF CACHE BOOL "" FORCE)
endforeach()
set(CLOCKWORK_PRODUCT_NAME "Sonic Pi" CACHE STRING "" FORCE)
add_subdirectory("${SP_ROOT}/app/external/supersonic" supersonic EXCLUDE_FROM_ALL)
# ── The language runtime ──────────────────────────────────────────────────
set(SP_MRUBY_DIR "${SP_BUILD}/mruby/${SP_MRUBY_TARGET}")
if(NOT EXISTS "${SP_MRUBY_DIR}/lib/libmruby.a")
    message(FATAL_ERROR "No mruby for ${SP_MRUBY_TARGET} in ${SP_BUILD}: run native/build-mruby.sh")
endif()
add_library(sp_mruby STATIC IMPORTED)
set_target_properties(sp_mruby PROPERTIES IMPORTED_LOCATION "${SP_MRUBY_DIR}/lib/libmruby.a")
# The defines and include paths mruby's headers need for this target (its flags file).
file(READ "${SP_MRUBY_DIR}/flags" SP_MRUBY_FLAGS)
string(REGEX MATCHALL "-D[^ \n]+" SP_MRUBY_DEFS "${SP_MRUBY_FLAGS}")
string(REGEX MATCHALL "-I[^ \n]+" SP_MRUBY_INCS "${SP_MRUBY_FLAGS}")
list(TRANSFORM SP_MRUBY_DEFS REPLACE "^-D" "")
list(TRANSFORM SP_MRUBY_INCS REPLACE "^-I" "")

add_library(sonicpi_core STATIC
    src/sonic_pi_core.cpp
    src/osc.cpp
    "${SP_ROOT}/app/web/runtime/host/sp_host.c"
    "${SP_BUILD}/runtime/runtime_irep.c")
target_include_directories(sonicpi_core PUBLIC src)
target_include_directories(sonicpi_core PRIVATE ${SP_MRUBY_INCS} "${SP_ROOT}/app/web/runtime/mruby/include"
    "${SP_ROOT}/app/external/supersonic/dsp/scsynth")
target_compile_definitions(sonicpi_core PRIVATE ${SP_MRUBY_DEFS})
target_link_libraries(sonicpi_core PUBLIC clockwork scsynth scsynth_tlsf sp_mruby m)
if(ANDROID)
    target_link_libraries(sonicpi_core PUBLIC log)
else()
    find_package(Threads REQUIRED)
    target_link_libraries(sonicpi_core PUBLIC Threads::Threads)
endif()

# ── The desktop test: programs rendered to audio, offline ─────────────────
if(NOT ANDROID)
    add_executable(sonic_pi_render_test test/render_test.cpp)
    target_link_libraries(sonic_pi_render_test PRIVATE sonicpi_core)
    target_compile_definitions(sonic_pi_render_test PRIVATE SP_ROOT="${SP_ROOT}")
    enable_testing()
    add_test(NAME render COMMAND sonic_pi_render_test)
endif()
