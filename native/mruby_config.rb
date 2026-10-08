# SPDX-License-Identifier: AGPL-3.0-or-later
# mruby for the Android app: the gem set Sonic Pi's runtime is built with
# (sonic-pi/app/web/runtime/build_config.rb), built for this machine and for
# each Android ABI asked for.
#
#   SP_RUNTIME=<sonic-pi>/app/web/runtime SP_ANDROID_ABIS=arm64-v8a,x86_64 \
#   ANDROID_NDK_HOME=<ndk> MRUBY_CONFIG=native/mruby_config.rb \
#   MRUBY_BUILD_DIR=<out> rake -C <sonic-pi>/app/web/runtime/mruby
#
# `host` is mrbc (to compile the runtime's Ruby to bytecode) and the library
# the desktop test of the native core links. native/build-mruby.sh runs this.
RUNTIME = ENV.fetch("SP_RUNTIME")
ANDROID_API = ENV.fetch("SP_ANDROID_API", "30")   # the app's minSdk: Android 11

COMMON_GEMS = lambda do |conf|
  conf.gembox "stdlib"
  conf.gembox "stdlib-ext"
  conf.gembox "math"
  conf.gembox "metaprog"          # instance_eval with a string: how a program runs
  conf.gem core: "mruby-rational"
  conf.gem core: "mruby-bigint"
  conf.gem File.join(RUNTIME, "mrbgems/sonic-pi-core")   # exact float printing
  conf.gem core: "mruby-bin-config"   # writes libmruby.flags.mak: the defines a target's headers need
end

MRuby::Build.new("host") do |conf|
  conf.toolchain
  COMMON_GEMS.call(conf)
  conf.gem core: "mruby-bin-mrbc"
  conf.cc.flags << "-fPIC"
end

ENV.fetch("SP_ANDROID_ABIS", "").split(",").map(&:strip).reject(&:empty?).each do |abi|
  MRuby::CrossBuild.new("android-#{abi}") do |conf|
    conf.toolchain :android, arch: abi, sdk_version: ANDROID_API.to_i
    COMMON_GEMS.call(conf)
    # Sonic Pi needs 64-bit integers (sample frames, NTP seconds); on a 32-bit
    # ABI that means unboxed values, as the wasm build does.
    conf.cc.defines << "MRB_INT64" << "MRB_NO_BOXING" if abi.start_with?("armeabi", "x86") && abi != "x86_64"
    conf.cc.flags << "-O2"
  end
end
