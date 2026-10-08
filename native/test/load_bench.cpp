// SPDX-License-Identifier: AGPL-3.0-or-later
// How hard a program works the core: it is run offline, as the phone runs it
// (the runtime ticked against the engine's clock, the engine rendered a
// device buffer at a time), and every render and every turn of the worker is
// timed. A render that takes longer than the audio it makes is a glitch on a
// device as fast as this machine; a turn of the worker longer than the
// program's schedule-ahead makes its sounds late.
//
//   sonic_pi_load_bench program.rb [seconds] [assets dir]
#include "sonic_pi_core.h"

#include <algorithm>
#include <chrono>
#include <cstdio>
#include <filesystem>
#include <fstream>
#include <sstream>
#include <string>
#include <vector>

namespace fs = std::filesystem;
using Clock = std::chrono::steady_clock;

namespace {
constexpr double kRate = 48000;
constexpr uint32_t kFrames = 192;   // a typical low-latency burst

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

double ms(Clock::duration d) { return std::chrono::duration<double, std::milli>(d).count(); }
}

int main(int argc, char** argv) {
    if (argc < 2) { std::fprintf(stderr, "usage: %s program.rb [seconds] [assets]\n", argv[0]); return 2; }
    std::ifstream in(argv[1]);
    std::stringstream code;
    code << in.rdbuf();
    const double seconds = argc > 2 ? std::atof(argv[2]) : 60;
    const std::string assets = argc > 3 ? argv[3] : makeAssets();

    std::vector<std::string> errors;
    sonicpi::Core core([&](const sonicpi::Event& e) {
        if (e.kind == sonicpi::Event::Error || e.kind == sonicpi::Event::Engine) errors.push_back(e.text);
    });
    sonicpi::Config config;
    config.assets = assets;
    config.sampleRate = kRate;
    std::string why;
    if (!core.boot(config, &why)) { std::fprintf(stderr, "boot: %s\n", why.c_str()); return 1; }

    std::vector<float> l(kFrames), r(kFrames);
    float* out[2] = {l.data(), r.data()};
    const double budget = kFrames / kRate * 1000;   // ms of audio per render
    // per second of music: the worst render and the worker's time
    std::vector<double> worstRender, workerMs;
    std::vector<double> renders;
    double worstTurn = 0, worstTurnAt = 0;

    core.run(code.str());
    const uint64_t blocks = uint64_t(seconds * kRate / kFrames);
    for (uint64_t b = 0; b < blocks; b++) {
        const double at = b * kFrames / kRate;
        const size_t sec = size_t(at);
        if (worstRender.size() <= sec) { worstRender.push_back(0); workerMs.push_back(0); }
        auto t0 = Clock::now();
        core.pump();
        auto t1 = Clock::now();
        core.render(out, 2, kFrames);
        auto t2 = Clock::now();
        const double turn = ms(t1 - t0), render = ms(t2 - t1);
        renders.push_back(render);
        worstRender[sec] = std::max(worstRender[sec], render);
        workerMs[sec] += turn;
        if (turn > worstTurn) { worstTurn = turn; worstTurnAt = at; }
    }

    std::vector<double> sorted = renders;
    std::sort(sorted.begin(), sorted.end());
    auto pct = [&](double p) { return sorted[std::min(sorted.size() - 1, size_t(p * sorted.size()))]; };
    double sum = 0;
    for (double v : renders) sum += v;
    std::printf("render of %u frames (%.2f ms of audio), over %.0f s:\n", kFrames, budget, seconds);
    std::printf("  mean %.3f ms (%.0f%% of real time)  p99 %.3f ms  p99.9 %.3f ms  worst %.3f ms (%.0f%%)\n",
                sum / renders.size(), 100 * sum / renders.size() / budget, pct(0.99), pct(0.999), sorted.back(),
                100 * sorted.back() / budget);
    std::printf("worker: worst turn %.1f ms at %.1f s\n", worstTurn, worstTurnAt);
    std::printf("per second: [second] worst render ms (%% of a burst), worker ms\n");
    for (size_t s = 0; s < worstRender.size(); s++)
        std::printf("  %3zu  %6.3f (%4.0f%%)  %7.1f\n", s, worstRender[s], 100 * worstRender[s] / budget, workerMs[s]);
    const auto st = core.stats();
    std::printf("sounds %llu, late %llu (worst %.0f ms), schedule held %llu times (%.0f ms in all)\n",
                (unsigned long long)st.sounds, (unsigned long long)st.late, st.worstLate * 1000,
                (unsigned long long)st.holds, st.held * 1000);
    for (const auto& e : errors) std::printf("  said: %s\n", e.c_str());
    return 0;
}
