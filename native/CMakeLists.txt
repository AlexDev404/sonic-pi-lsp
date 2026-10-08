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
# scsynth schedules its own bundles), Link or plugin hosting.
foreach(opt CLOCKWORK_DEVICE CLOCKWORK_MIDI CLOCKWORK_GAMEPAD CLOCKWORK_OSC CLOCKWORK_SCHEDULER
            CLOCKWORK_LINK CLOCKWORK_LINK_AUDIO CLOCKWORK_PLUGINS CLOCKWORK_STANDALONE BUILD_TESTS
            CLOCKWORK_RUST)
    set(${opt} OFF CACHE BOOL "" FORCE)
endforeach()
set(CLOCKWORK_PRODUCT_NAME "Sonic Pi" CACHE STRING "" FORCE)

# The few changes the upstream sources need (patches/*.patch, each applied in
# the submodule it is named for): Android's libc has no librt or POSIX shared
# memory, and clockwork links the Rust core built below. Applied once; a
# patch already in place is left as it is.
set(SP_PATCHES clockwork-android "app/external/supersonic/clockwork")
while(SP_PATCHES)
    list(POP_FRONT SP_PATCHES sp_patch sp_patch_dir)
    set(sp_patch_file "${CMAKE_CURRENT_SOURCE_DIR}/../patches/${sp_patch}.patch")
    execute_process(COMMAND git apply --reverse --check "${sp_patch_file}"
        WORKING_DIRECTORY "${SP_ROOT}/${sp_patch_dir}" RESULT_VARIABLE sp_applied OUTPUT_QUIET ERROR_QUIET)
    if(NOT sp_applied EQUAL 0)
        execute_process(COMMAND git apply "${sp_patch_file}"
            WORKING_DIRECTORY "${SP_ROOT}/${sp_patch_dir}" RESULT_VARIABLE sp_failed)
        if(sp_failed)
            message(FATAL_ERROR "patches/${sp_patch}.patch does not apply to ${sp_patch_dir}")
        endif()
        message(STATUS "Applied patches/${sp_patch}.patch")
    endif()
endwhile()

# clockwork's core is partly Rust (its heap, sinks, ports, timelines). Its own
# cargo step builds for the machine it runs on, or for wasm; this one builds
# for the target, Android's triple included, into this build directory, and
# clockwork links that archive (CLOCKWORK_RUST_LIB).
find_program(SP_CARGO cargo HINTS "$ENV{HOME}/.cargo/bin" REQUIRED)
set(SP_RUST_ENV "CARGO_TARGET_DIR=${CMAKE_CURRENT_BINARY_DIR}/rust")
set(SP_RUST_FLAGS build --release --locked --no-default-features -p clockwork-native)
if(ANDROID)
    set(SP_RUST_TRIPLES_arm64-v8a aarch64-linux-android)
    set(SP_RUST_TRIPLES_x86_64 x86_64-linux-android)
    set(SP_RUST_TRIPLES_armeabi-v7a armv7-linux-androideabi)
    set(SP_RUST_TRIPLES_x86 i686-linux-android)
    set(SP_RUST_TRIPLE "${SP_RUST_TRIPLES_${ANDROID_ABI}}")
    if(NOT SP_RUST_TRIPLE)
        message(FATAL_ERROR "No Rust target for ${ANDROID_ABI}")
    endif()
    string(REPLACE "-" "_" sp_triple_env "${SP_RUST_TRIPLE}")
    string(TOUPPER "${sp_triple_env}" sp_triple_upper)
    # any C a crate compiles, with the NDK's compiler for this ABI
    list(APPEND SP_RUST_ENV "CC_${sp_triple_env}=${CMAKE_C_COMPILER}"
        "CFLAGS_${sp_triple_env}=--target=${CMAKE_C_COMPILER_TARGET}"
        "AR_${sp_triple_env}=${CMAKE_AR}"
        "CARGO_TARGET_${sp_triple_upper}_LINKER=${CMAKE_C_COMPILER}")
    list(APPEND SP_RUST_FLAGS --target ${SP_RUST_TRIPLE})
    set(SP_RUST_LIB "${CMAKE_CURRENT_BINARY_DIR}/rust/${SP_RUST_TRIPLE}/release/libclockwork_native.a")
else()
    set(SP_RUST_LIB "${CMAKE_CURRENT_BINARY_DIR}/rust/release/libclockwork_native.a")
endif()
add_custom_target(sp_clockwork_rust_build
    COMMAND ${CMAKE_COMMAND} -E env ${SP_RUST_ENV} "${SP_CARGO}" ${SP_RUST_FLAGS}
    WORKING_DIRECTORY "${SP_ROOT}/app/external/supersonic/clockwork/rust"
    BYPRODUCTS "${SP_RUST_LIB}"
    COMMENT "clockwork's Rust core (cargo${SP_RUST_TRIPLE})"
    VERBATIM)
set(CLOCKWORK_RUST_LIB "${SP_RUST_LIB}" CACHE FILEPATH "" FORCE)
add_subdirectory("${SP_ROOT}/app/external/supersonic" supersonic EXCLUDE_FROM_ALL)
add_dependencies(clockwork sp_clockwork_rust_build)

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
    target_link_libraries(sonicpi_core PUBLIC log dl)
else()
    find_package(Threads REQUIRED)
    target_link_libraries(sonicpi_core PUBLIC Threads::Threads)
endif()

# ── The desktop test: programs rendered to audio, offline ─────────────────
if(NOT ANDROID)
    add_executable(sonic_pi_render_test test/render_test.cpp)
    target_link_libraries(sonic_pi_render_test PRIVATE sonicpi_core)
    target_compile_definitions(sonic_pi_render_test PRIVATE SP_ROOT="${SP_ROOT}")
    add_executable(sonic_pi_load_bench test/load_bench.cpp)
    target_link_libraries(sonic_pi_load_bench PRIVATE sonicpi_core)
    target_compile_definitions(sonic_pi_load_bench PRIVATE SP_ROOT="${SP_ROOT}")
    enable_testing()
    add_test(NAME render COMMAND sonic_pi_render_test)
endif()
