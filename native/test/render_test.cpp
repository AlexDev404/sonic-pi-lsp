// SPDX-License-Identifier: AGPL-3.0-or-later
// Sonic Pi programs, run through the native core and rendered offline: the
// runtime ticks against the engine's clock as it would on a phone, and the
// test hears what came out. Each case checks the audio is there (or gone,
// after a Stop) and what the log said. Writes each render as a WAV beside it.
//
//   sonic_pi_render_test [assets dir] [out dir]
//
// The assets dir holds synthdefs/, samples/, buffers/ and piano_wavetable.dat;
// by default one is made from the Sonic Pi checkout, as the app's is.
#include "sonic_pi_core.h"

#include <cmath>
#include <cstdio>
#include <cstring>
#include <filesystem>
#include <fstream>
#include <functional>
#include <string>
#include <vector>

namespace fs = std::filesystem;
using sonicpi::Core;
using sonicpi::Event;

namespace {

constexpr double kRate = 48000;
constexpr uint32_t kFrames = 256;   // a device buffer that is not the engine's block

std::string makeAssets() {
    const fs::path root = SP_ROOT;
    const fs::path dir = fs::temp_directory_path() / "sonic-pi-render-assets";
    fs::create_directories(dir);
    auto link = [&](const fs::path& from, const char* name) {
        const fs::path to = dir / name;
        std::error_code ec;
        fs::remove(to, ec);
        fs::create_symlink(from, to);
    };
    link(root / "etc/synthdefs/compiled", "synthdefs");
    link(root / "etc/samples", "samples");
    link(root / "etc/buffers", "buffers");
    link(root / "app/external/piano/piano_wavetable.dat", "piano_wavetable.dat");
    return dir.string();
}

void writeWav(const std::string& path, const std::vector<float>& left, const std::vector<float>& right) {
    std::ofstream f(path, std::ios::binary);
    const uint32_t frames = uint32_t(left.size()), data = frames * 4;
    auto u32 = [&](uint32_t v) { f.write(reinterpret_cast<const char*>(&v), 4); };
    auto u16 = [&](uint16_t v) { f.write(reinterpret_cast<const char*>(&v), 2); };
    f.write("RIFF", 4); u32(36 + data); f.write("WAVEfmt ", 8);
    u32(16); u16(1); u16(2); u32(uint32_t(kRate)); u32(uint32_t(kRate) * 4); u16(4); u16(16);
    f.write("data", 4); u32(data);
    for (uint32_t i = 0; i < frames; i++) {
        for (float s : {left[i], right[i]}) {
            const int16_t v = int16_t(std::lround(std::clamp(s, -1.0f, 1.0f) * 32767));
            f.write(reinterpret_cast<const char*>(&v), 2);
        }
    }
}

struct Render {
    std::vector<float> left, right;
    std::vector<Event> events;

    // RMS over [from, to) seconds
    double rms(double from, double to) const {
        const size_t a = size_t(from * kRate), b = std::min(left.size(), size_t(to * kRate));
        double sum = 0;
        for (size_t i = a; i < b; i++) sum += double(left[i]) * left[i] + double(right[i]) * right[i];
        return b > a ? std::sqrt(sum / double(2 * (b - a))) : 0;
    }
    bool said(Event::Kind kind, const std::string& part) const {
        for (const auto& e : events) if (e.kind == kind && e.text.find(part) != std::string::npos) return true;
        return false;
    }
};

// Boots a core, runs `code`, renders `seconds`, calling `at` (if given) once
// `atSecond` has been rendered.
Render render(Core& core, Render& r, const std::string& code, double seconds,
              double atSecond = -1, const std::function<void()>& at = {}) {
    core.run(code);
    std::vector<float> l(kFrames), rr(kFrames);
    float* out[2] = {l.data(), rr.data()};
    const size_t total = size_t(seconds * kRate);
    bool fired = false;
    for (size_t done = 0; done < total; done += kFrames) {
        if (!fired && at && done >= size_t(atSecond * kRate)) { at(); fired = true; }
        core.pump();
        core.render(out, 2, kFrames);
        r.left.insert(r.left.end(), l.begin(), l.end());
        r.right.insert(r.right.end(), rr.begin(), rr.end());
    }
    return r;
}

int failures = 0;
void check(bool ok, const std::string& what) {
    std::printf("  %s %s\n", ok ? "ok  " : "FAIL", what.c_str());
    if (!ok) failures++;
}

} // namespace

int main(int argc, char** argv) {
    const std::string assets = argc > 1 ? argv[1] : makeAssets();
    const fs::path outDir = argc > 2 ? argv[2] : fs::path(".");

    Render r;
    Core core([&](const Event& e) {
        r.events.push_back(e);
        static const char* names[] = {"output", "log", "error", "cue", "synth", "state", "engine"};
        std::printf("    [%s] %s%s\n", names[e.kind], e.thread.empty() ? "" : (e.thread + ": ").c_str(), e.text.c_str());
    });
    sonicpi::Config config;
    config.assets = assets;
    config.sampleRate = kRate;
    std::string err;
    if (!core.boot(config, &err)) {
        std::printf("boot failed: %s\n", err.c_str());
        return 1;
    }

    std::printf("a melody, a sample, fx, a live loop and puts\n");
    render(core, r, R"(
use_bpm 120
puts "hello from the runtime"
live_loop :drums do
  sample :bd_haus, amp: 1.5
  sleep 1
end
with_fx :reverb, room: 0.8 do
  play_pattern_timed [60, 64, 67, 72], [0.5], release: 0.4
end
use_synth :prophet
play chord(:e3, :minor), release: 2
)", 6.0);
    writeWav((outDir / "render-melody.wav").string(), r.left, r.right);
    check(r.said(Event::Output, "hello from the runtime"), "puts reached the log");
    check(r.said(Event::Synth, "sample :bd_haus"), "the sample was logged");
    check(r.said(Event::Synth, "synth :prophet"), "use_synth took effect");
    check(r.rms(0.5, 3.0) > 0.01, "audible: rms " + std::to_string(r.rms(0.5, 3.0)));
    check(!r.said(Event::Error, ""), "no errors");

    std::printf("Stop fades everything out\n");
    const size_t before = r.left.size();
    Render s;
    std::swap(s.events, r.events);
    r.left.clear(); r.right.clear();
    render(core, r, "live_loop :pad do\n  synth :dsaw, note: :e2, sustain: 4\n  sleep 4\nend\n", 5.0,
           1.5, [&] { core.stop(); });
    writeWav((outDir / "render-stop.wav").string(), r.left, r.right);
    check(r.rms(0.6, 1.4) > 0.01, "playing before the stop: rms " + std::to_string(r.rms(0.6, 1.4)));
    check(r.rms(3.5, 5.0) < 0.0005, "silent after the fade: rms " + std::to_string(r.rms(3.5, 5.0)));
    check(r.said(Event::State, "stopped"), "the session said it stopped");
    (void)before;

    std::printf("errors are reported, not fatal\n");
    r.left.clear(); r.right.clear();
    render(core, r, "play 60\nnot_a_sonic_pi_fn 1\n", 0.5);
    check(r.said(Event::Error, "not_a_sonic_pi_fn"), "an unknown function is an error naming it");
    render(core, r, "sample :ambi_choir\n", 2.0);
    check(r.rms(0.6, 2.4) > 0.005, "and the next run still plays: rms " + std::to_string(r.rms(0.6, 2.4)));

    std::printf("synths preloaded from boot: a new one plays without holding the schedule\n");
    r.left.clear(); r.right.clear();
    r.events.clear();
    render(core, r, "sleep 2\n", 2.5);   // the preloads finish
    r.left.clear(); r.right.clear();
    r.events.clear();
    render(core, r, "use_synth :hollow\nplay 60, release: 1\n", 1.5);
    check(r.rms(0.5, 1.5) > 0.002, "audible: rms " + std::to_string(r.rms(0.5, 1.5)));
    check(!r.said(Event::Engine, "Waited"), "nothing waited for a load");

    std::printf(":piano, its synthdef preloaded, still gets its table\n");
    r.left.clear(); r.right.clear();
    render(core, r, "use_synth :piano\nplay 60\n", 2.0);
    check(r.rms(0.5, 2.0) > 0.002, "audible: rms " + std::to_string(r.rms(0.5, 2.0)));

    std::printf("%s\n", failures ? "FAILED" : "all passed");
    return failures ? 1 : 0;
}
