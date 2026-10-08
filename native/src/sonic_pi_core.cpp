// SPDX-License-Identifier: AGPL-3.0-or-later
#include "sonic_pi_core.h"
#include "osc.h"

#include "clockwork_embed.h"
#include "clockwork_client.h"
#include "clockwork_asset_pool.h"
#include "clockwork_audio_file.h"
#include "dsp_api.h"
#include "lanes/lanes.h"
#include "audio_processor.h"
#include "scsynth_options.h"

#include <sys/mman.h>
#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <dirent.h>
#include <fstream>
#include <sstream>

#if defined(__aarch64__)
#define SP_ARM_FTZ() do { uint64_t fpcr; __asm__ volatile("mrs %0, fpcr" : "=r"(fpcr)); \
    if (!(fpcr & (1ull << 24))) __asm__ volatile("msr fpcr, %0" :: "r"(fpcr | (1ull << 24))); } while (0)
#elif defined(__x86_64__) || defined(__i386__)
#include <xmmintrin.h>
#include <pmmintrin.h>
#define SP_ARM_FTZ() do { _MM_SET_FLUSH_ZERO_MODE(_MM_FLUSH_ZERO_ON); _MM_SET_DENORMALS_ZERO_MODE(_MM_DENORMALS_ZERO_ON); } while (0)
#else
#define SP_ARM_FTZ() do {} while (0)
#endif

// The runtime's seam (sonic-pi/app/web/runtime/host/sp_host.c).
extern "C" {
int sp_init(void);
int sp_install_table(const char* source, const uint8_t* bytes, int len);
int sp_set_samples_dir(const char* dir);
int sp_install_sample(const char* path, int frames, int chans, int rate, const double* onsets, int n);
int sp_live_boot(void);
int sp_run(const char* code, double now);
double sp_tick(double now);
void sp_stop_all(void);
void sp_silence(double fade, double since, double now);
void sp_hold(double seconds);
const uint8_t* sp_out_ptr(void);
int sp_out_len(void);
uint32_t supersonic_buffer_guard_before(void);
uint32_t supersonic_buffer_guard_after(void);
}

namespace sonicpi {

namespace {

constexpr uint32_t kOrigin = 1;            // the token our messages carry, so the engine answers us
constexpr double kStopFade = 1.0;          // seconds a Stop fades over (the web app's STOP_FADE)
constexpr double kLateMargin = 0.05;       // a held-back sound goes this far ahead of the clock at least
constexpr uint32_t kFrameSound = 1, kFrameHost = 2, kFrameGui = 3;

// Sonic Pi's random streams (the white one is also the studio's rand_buf).
const std::pair<const char*, const char*> kTables[] = {
    {"white", "rand-stream.wav"}, {"pink", "rand-stream-pink.wav"}, {"light_pink", "rand-stream-light-pink.wav"},
    {"dark_pink", "rand-stream-dark-pink.wav"}, {"perlin", "rand-stream-perlin.wav"},
};

double ntpNow() {
    return std::chrono::duration<double>(std::chrono::system_clock::now().time_since_epoch()).count() + 2208988800.0;
}

int64_t steadyNs() {
    return std::chrono::duration_cast<std::chrono::nanoseconds>(std::chrono::steady_clock::now().time_since_epoch()).count();
}

bool readFile(const std::string& path, std::vector<uint8_t>& out) {
    std::ifstream f(path, std::ios::binary);
    if (!f) return false;
    out.assign(std::istreambuf_iterator<char>(f), {});
    return true;
}

uint32_t le32(const uint8_t* p) { return uint32_t(p[0]) | uint32_t(p[1]) << 8 | uint32_t(p[2]) << 16 | uint32_t(p[3]) << 24; }
uint32_t be32(const uint8_t* p) { return uint32_t(p[0]) << 24 | uint32_t(p[1]) << 16 | uint32_t(p[2]) << 8 | p[3]; }

std::string baseName(const std::string& path) {
    const size_t slash = path.find_last_of('/');
    return slash == std::string::npos ? path : path.substr(slash + 1);
}

std::string stem(const std::string& path) {
    std::string b = baseName(path);
    const size_t dot = b.find_last_of('.');
    return dot == std::string::npos ? b : b.substr(0, dot);
}

bool endsWith(const std::string& s, const char* tail) {
    const size_t n = std::strlen(tail);
    return s.size() >= n && s.compare(s.size() - n, n, tail) == 0;
}

// A bundle's NTP timetag (bytes 8..16), in seconds; 0 for "immediately".
double bundleTime(const std::vector<uint8_t>& b) {
    if (b.size() < 16) return 0;
    const uint32_t secs = be32(&b[8]), frac = be32(&b[12]);
    return secs == 0 ? 0 : secs + frac / 4294967296.0;
}

void retime(std::vector<uint8_t>& b, double shift) {
    const double t = bundleTime(b);
    if (t == 0) return;
    const double moved = t + shift, whole = std::floor(moved);
    const uint32_t secs = uint32_t(whole);
    const uint32_t frac = uint32_t(std::min(4294967295.0, std::round((moved - whole) * 4294967296.0)));
    const uint8_t bytes[8] = {uint8_t(secs >> 24), uint8_t(secs >> 16), uint8_t(secs >> 8), uint8_t(secs),
                              uint8_t(frac >> 24), uint8_t(frac >> 16), uint8_t(frac >> 8), uint8_t(frac)};
    std::memcpy(&b[8], bytes, 8);
}

// A record's opts, as Sonic Pi's log prints them: {note: 60.0, release: 0.5}
std::string optsText(const osc::Value& pairs, const char* skip) {
    std::string s = "{";
    bool first = true;
    for (size_t i = 0; i + 1 < pairs.items.size(); i += 2) {
        const std::string key = pairs.items[i].str();
        if (skip && key == skip) continue;
        s += (first ? "" : ", ") + key + ": " + pairs.items[i + 1].str();
        first = false;
    }
    return s + "}";
}

const osc::Value* optValue(const osc::Value& pairs, const char* key) {
    for (size_t i = 0; i + 1 < pairs.items.size(); i += 2)
        if (pairs.items[i].type == osc::Value::String && pairs.items[i].text == key) return &pairs.items[i + 1];
    return nullptr;
}

} // namespace

Core::Core(Listener listener) : mListener(std::move(listener)) {}

Core::~Core() {
    shutdown();
    if (mEmbed) clockwork_embed_close(mEmbed);
    clockwork_detach();   // the attachment boot() made itself
    if (mPool) clockwork_asset_pool_close(mPool);
    if (mInbox) munmap(mInbox, mInboxBytes);
}

void Core::emit(Event::Kind kind, std::string text, int job, double time, int line, std::string thread) {
    if (!mListener) return;
    Event e;
    e.kind = kind;
    e.text = std::move(text);
    e.job = job;
    e.time = time;
    e.line = line;
    e.thread = std::move(thread);
    mListener(e);
}

// ── Boot ──────────────────────────────────────────────────────────────────

bool Core::boot(const Config& config, std::string* error) {
    auto fail = [&](const std::string& why) { if (error) *error = why; return false; };
    if (mEmbed) return fail("already booted");
    mConfig = config;

    // The runtime: its Ruby, the random tables, and the built-in samples it may name.
    if (sp_init() != 0) return fail("the language runtime did not start");
    for (const auto& [source, file] : kTables) {
        std::vector<uint8_t> wav;
        if (!readFile(config.assets + "/buffers/" + file, wav)) return fail(std::string("missing random table ") + file);
        if (sp_install_table(source, wav.data(), int(wav.size())) != 0) return fail(std::string("bad random table ") + file);
    }
    const std::string samples = config.assets + "/samples";
    sp_set_samples_dir(samples.c_str());
    if (DIR* d = opendir(samples.c_str())) {
        while (dirent* ent = readdir(d)) {
            const std::string name = ent->d_name;
            if (!endsWith(name, ".flac") && !endsWith(name, ".wav")) continue;
            const std::string path = samples + "/" + name;
            ClockworkAudioInfo info{};
            info.struct_bytes = sizeof info;
            if (clockwork_audio_probe_file(path.c_str(), &info) != CLOCKWORK_OK) continue;
            sp_install_sample(path.c_str(), int(info.frames), int(info.channels), int(info.sample_rate), nullptr, 0);
        }
        closedir(d);
    } else {
        return fail("missing samples in " + samples);
    }
    if (sp_live_boot() != 0) return fail("the live session did not start");

    // The engine. Attached first through the lanes, which is where scsynth's
    // options and the inbox (where decoded samples live) are given; the embed
    // handle then joins that engine for its render FIFOs and its client.
    mInboxBytes = size_t(config.inboxMB) << 20;
    void* inbox = mmap(nullptr, mInboxBytes, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    if (inbox == MAP_FAILED) return fail("no memory for samples");
    mInbox = static_cast<uint8_t*>(inbox);
    std::ostringstream guest;
    guest << "realTimeMemorySize=" << config.realTimeMemoryKB << "\n"
          << "numAudioBusChannels=1024\n"
          << "numBuffers=4096\n"
          << "maxNodes=4096\n"
          << "maxGraphDefs=1024\n";
    const std::string guestText = guest.str();
    clockwork_declare_fp_env(DSP_FP_ENV_FLUSH_TO_ZERO);   // render() arms it on the audio thread
    // scsynth takes its real-time pool from clockwork's heap, sized once per
    // build: the pool and scsynth's headroom (scsynth_options.h, scsynth_heap_bytes).
    clockwork_set_heap_bytes(size_t(scsynth_heap_bytes(config.realTimeMemoryKB)));
    const int rc = clockwork_attach(config.sampleRate, config.blockSize, 0, config.outputChannels, 0,
                                    nullptr, 0, guestText.data(), uint32_t(guestText.size()), 0,
                                    mInbox, uint32_t(mInboxBytes), nullptr, 0);
    if (rc == CLOCKWORK_ATTACH_REFUSED) return fail("the engine refused to start");
    if (const char* why = clockwork_boot_error(); why && *why) return fail(std::string("the engine did not start: ") + why);

    ClockworkEmbedConfig ec{};
    ec.struct_bytes = sizeof ec;
    ec.sample_rate = config.sampleRate;
    ec.block_size = config.blockSize;
    ec.output_channels = config.outputChannels;
    ec.max_render_frames = config.maxRenderFrames;
    ClockworkStatus st = CLOCKWORK_OK;
    mEmbed = clockwork_embed_attach(&ec, &st);
    if (!mEmbed) return fail(std::string("the engine's client did not open: ") + clockwork_client_status_text(st));
    mClient = clockwork_embed_client(mEmbed);
    mSampleRate = clockwork_embed_sample_rate(mEmbed);
    mPool = clockwork_asset_pool_open(uint32_t(mInboxBytes));
    if (!mPool) return fail("no memory for the sample pool");

    mClockNtp.store(ntpNow());
    mClockWallNs.store(steadyNs());
    emit(Event::Engine, "Sonic Pi is ready: " + std::to_string(int(mSampleRate)) + " Hz");
    return true;
}

// ── The worker ────────────────────────────────────────────────────────────

void Core::start() {
    if (mThread.joinable() || !mEmbed) return;
    mQuit = false;
    mThread = std::thread([this] { loop(); });
}

void Core::shutdown() {
    {
        std::lock_guard<std::mutex> lock(mMutex);
        mQuit = true;
    }
    mWake.notify_all();
    if (mThread.joinable()) mThread.join();
}

void Core::run(const std::string& code) {
    {
        std::lock_guard<std::mutex> lock(mMutex);
        mCommands.push_back({Command::Run, code});
    }
    mWake.notify_all();
}

void Core::stop() {
    {
        std::lock_guard<std::mutex> lock(mMutex);
        mCommands.push_back({Command::Stop, {}});
    }
    mWake.notify_all();
}

void Core::loop() {
    while (true) {
        turn(false);
        std::unique_lock<std::mutex> lock(mMutex);
        if (mQuit) break;
        if (!mCommands.empty()) continue;
        // Until the runtime's next wake, but often enough to hear the engine's replies.
        double wait = 0.005;
        if (mSessionRunning && mNextWake >= 0) wait = std::clamp(mNextWake - engineNow(), 0.0, wait);
        if (!mWaiting.empty() || !mDefQueue.empty() || !mSampleQueue.empty()) wait = std::min(wait, 0.002);
        mWake.wait_for(lock, std::chrono::duration<double>(wait));
        if (mQuit) break;
    }
}

void Core::pump() { turn(true); }

void Core::turn(bool) {
    handleCommands();
    pollEngine();
    processLoads();
    // Sounds held back for a load go, in order, once what they need is in.
    while (!mWaiting.empty() && ready(mWaiting.front())) {
        Sound s = std::move(mWaiting.front());
        mWaiting.pop_front();
        // A load took so long that the sound would now be late: the whole
        // schedule moves on by the time lost, as the web app's run gate does,
        // so the music starts together rather than in pieces.
        const double t = bundleTime(s.bundle), now = engineNow();
        double shift = 0;
        if (t > 0 && t < now + kLateMargin) {
            shift = now + kLateMargin - t + 0.1;
            sp_hold(shift);
            for (auto& w : mWaiting) retime(w.bundle, shift);
            if (mNextWake >= 0) mNextWake += shift;
        }
        sendSound(s, shift);
    }
    if (mSessionRunning && mNextWake >= 0 && mWaiting.empty() && engineNow() >= mNextWake) tick();
}

void Core::handleCommands() {
    std::deque<Command> commands;
    {
        std::lock_guard<std::mutex> lock(mMutex);
        commands.swap(mCommands);
    }
    for (auto& c : commands) {
        if (c.what == Command::Run) {
            const double now = engineNow();
            if (mT0 < 0) mT0 = now;
            const int job = sp_run(c.code.c_str(), now);
            drain();
            if (job < 0) { emit(Event::Error, "The program did not start."); continue; }
            if (!mSessionRunning.exchange(true)) emit(Event::State, "running");
            mNextWake = now;
            tick();
        } else {
            const double since = engineNow();
            sp_stop_all();
            mWaiting.clear();
            drain();
            clear_scheduler();             // what the engine still holds scheduled goes
            sp_silence(kStopFade, since, engineNow());
            drain();
            mNextWake = -1;
            if (mSessionRunning.exchange(false)) emit(Event::State, "stopped");
        }
    }
}

void Core::tick() {
    const double next = sp_tick(engineNow());
    drain();
    mNextWake = next;
    if (next < 0 && mSessionRunning.exchange(false)) emit(Event::State, "stopped");
}

// What the runtime's last call left in its outbox: sounds, word on what to
// load, and records for the UI (sp_host.c, "The audio stream").
void Core::drain() {
    const int len = sp_out_len();
    if (len <= 0) return;
    const std::vector<uint8_t> out(sp_out_ptr(), sp_out_ptr() + len);
    std::vector<Sound> sounds;
    for (size_t at = 0; at + 8 <= out.size();) {
        const uint32_t size = le32(&out[at]), kind = le32(&out[at + 4]);
        const uint8_t* p = &out[at + 8];
        if (at + 8 + size > out.size()) break;
        if (kind == kFrameSound && size >= 36) {
            // /sonic-pi/sound ,iib synthdef buffer bundle: at fixed offsets
            const int32_t def = int32_t(be32(p + 24)), buf = int32_t(be32(p + 28));
            const uint32_t bytes = be32(p + 32);
            if (36 + bytes <= size) {
                Sound s;
                s.bundle.assign(p + 36, p + 36 + bytes);
                if (def >= 0 && size_t(def) < mSynthdefNames.size()) s.synthdef = mSynthdefNames[size_t(def)];
                s.buffer = buf;
                sounds.push_back(std::move(s));
            }
        } else if (kind == kFrameHost) {
            osc::Parsed m;
            if (osc::parse(p, size, m) && m.args.size() >= 2) {
                const int32_t n = int32_t(m.args[0].integer);
                const std::string& name = m.args[1].text;
                if (m.address == "/sonic-pi/synthdef") {
                    if (n >= 0) {
                        if (mSynthdefNames.size() <= size_t(n)) mSynthdefNames.resize(size_t(n) + 1);
                        mSynthdefNames[size_t(n)] = name;
                    }
                    requestSynthdef(name);
                } else if (m.address == "/sonic-pi/sample") {
                    requestSample(n, name);
                } else if (m.address == "/sonic-pi/sample_free") {
                    freeSample(n);
                } else if (m.address == "/sonic-pi/synthdef-url") {
                    emit(Event::Error, "load_synthdef is not available in this app yet: " + name);
                }
            }
        } else if (kind == kFrameGui) {
            record(p, size);
        }
        at += 8 + size;
    }
    for (auto& s : sounds) {
        if (mWaiting.empty() && ready(s)) sendSound(s, 0);
        else mWaiting.push_back(std::move(s));
    }
}

bool Core::ready(const Sound& s) const {
    if (!s.synthdef.empty()) {
        if (!mDefsReady.count(s.synthdef)) return false;
        if (s.synthdef == "sonic-pi-piano" && !mPianoLoaded) return false;
    }
    // a buffer never asked for was loaded before it was named: ready
    return s.buffer < 0 || mBuffersReady.count(s.buffer) || !mBufferAsked.count(s.buffer);
}

bool Core::send(const std::vector<uint8_t>& osc) {
    for (int tries = 0; tries < 200; tries++) {
        const ClockworkStatus st = clockwork_client_send(mClient, osc.data(), uint32_t(osc.size()), kOrigin);
        if (st == CLOCKWORK_OK) return true;
        if (st != CLOCKWORK_E_FULL) {
            emit(Event::Engine, std::string("the engine refused a message: ") + clockwork_client_status_text(st));
            return false;
        }
        std::this_thread::sleep_for(std::chrono::milliseconds(1));   // the ring drains every block
    }
    emit(Event::Engine, "the engine is not taking messages");
    return false;
}

void Core::sendSound(Sound& s, double) { send(s.bundle); }

// ── Loads ─────────────────────────────────────────────────────────────────

void Core::requestSynthdef(const std::string& name) {
    if (name.empty() || mDefsReady.count(name) || mDefsAsked.count(name)) return;
    mDefsAsked.insert(name);
    mDefQueue.push_back(name);
    if (name == "sonic-pi-piano" && !mPianoLoaded) loadPianoTable();
}

void Core::requestSample(int32_t bufnum, const std::string& file) {
    if (bufnum < 0 || mBuffersReady.count(bufnum) || mBufferAsked.count(bufnum)) return;
    mBufferAsked[bufnum] = file;
    mSampleQueue.emplace_back(bufnum, file);
}

void Core::freeSample(int32_t bufnum) {
    send(osc::Message("/b_free").i(bufnum).bytes());
    mBuffersReady.erase(bufnum);
    mBufferAsked.erase(bufnum);
}

void Core::processLoads() {
    if (!mDefInFlight) sendNextSynthdef();
    while (!mSampleQueue.empty()) {
        auto [bufnum, file] = mSampleQueue.front();
        mSampleQueue.pop_front();
        const std::string path = resolveSample(file);
        if (!loadSampleNow(bufnum, path)) {
            mBufferAsked.erase(bufnum);
            mBuffersReady.insert(bufnum);   // its sounds go, silent, rather than hold up everything after them
        }
    }
}

// One synthdef with the engine at a time, so no block parses more than one.
void Core::sendNextSynthdef() {
    while (!mDefQueue.empty()) {
        const std::string name = mDefQueue.front();
        mDefQueue.pop_front();
        std::vector<uint8_t> bytes;
        if (!readFile(mConfig.assets + "/synthdefs/" + name + ".scsyndef", bytes)) {
            emit(Event::Error, "The synth " + name + " is not in this app.");
            mDefsReady.insert(name);    // its sounds go, and the engine says it has no such synth
            continue;
        }
        mDefInFlight = true;
        mDefInFlightName = name;
        send(osc::Message("/d_recv").b(bytes.data(), bytes.size()).bytes());
        return;
    }
}

std::string Core::resolveSample(const std::string& file) const {
    if (FILE* f = std::fopen(file.c_str(), "rb")) { std::fclose(f); return file; }
    const std::string name = baseName(file);
    for (const auto& [source, table] : kTables) if (name == table) return mConfig.assets + "/buffers/" + name;
    return mConfig.assets + "/samples/" + name;
}

// Decoded into the inbox with scsynth's guard frames either side, then handed
// over as an asset bound to the buffer number (as SuperSonic's own front does
// for /b_allocRead: sonic-pi/app/external/supersonic/front/SuperSonicFront.cpp).
bool Core::loadSampleNow(int32_t bufnum, const std::string& path) {
    ClockworkAudioInfo info{};
    info.struct_bytes = sizeof info;
    float* samples = nullptr;
    if (clockwork_audio_decode_file(path.c_str(), &info, &samples) != CLOCKWORK_OK || !samples || info.frames == 0) {
        clockwork_audio_free(samples);
        emit(Event::Error, "Could not load the sample " + stem(path) + ".");
        return false;
    }
    const uint64_t frameBytes = uint64_t(info.channels) * sizeof(float);
    const uint32_t before = supersonic_buffer_guard_before(), after = supersonic_buffer_guard_after();
    const uint64_t slotBytes = (before + info.frames + after) * frameBytes;
    uint32_t slot = 0;
    if (slotBytes > 0xFFFFFFFFull || clockwork_asset_pool_alloc(mPool, uint32_t(std::max<uint64_t>(slotBytes, 16)), &slot) != 0) {
        clockwork_audio_free(samples);
        emit(Event::Error, "No room for the sample " + stem(path) + ".");
        return false;
    }
    std::memset(mInbox + slot, 0, size_t(slotBytes));
    const uint32_t payload = slot + uint32_t(before * frameBytes);
    std::memcpy(mInbox + payload, samples, size_t(info.frames * frameBytes));
    clockwork_audio_free(samples);
    mBufferSlots[bufnum] = slot;
    send(osc::Message("/b_free").i(bufnum).bytes());
    send(osc::Message("/clockwork/asset/commit")
             .i(bufnum).i(CLOCKWORK_ASSET_AUDIO_F32).i(int32_t(payload)).i(int32_t(info.frames * frameBytes))
             .i(int32_t(info.channels)).i(int32_t(info.frames)).f(float(info.sample_rate))
             .bytes());
    return true;
}

// :piano's synthdef needs MdaPiano's sample table too, handed to the plugin
// through a buffer it copies from (sonic-pi/app/web/web/sonic_pi.js pianoTable).
bool Core::loadPianoTable() {
    std::vector<uint8_t> raw;
    if (!readFile(mConfig.assets + "/piano_wavetable.dat", raw) || raw.size() < 2) {
        emit(Event::Error, "The piano's sample table is missing: :piano will be silent.");
        mPianoLoaded = true;
        return false;
    }
    const uint32_t frames = uint32_t(raw.size() / 2);
    uint32_t slot = 0;
    if (clockwork_asset_pool_alloc(mPool, frames * 4, &slot) != 0) { mPianoLoaded = true; return false; }
    auto* dst = reinterpret_cast<float*>(mInbox + slot);
    for (uint32_t i = 0; i < frames; i++) dst[i] = float(int16_t(raw[2 * i] | raw[2 * i + 1] << 8)) / 32768.0f;
    const int32_t bufnum = mNextOwnBuffer++;
    mBufferSlots[bufnum] = slot;
    send(osc::Message("/clockwork/asset/commit")
             .i(bufnum).i(CLOCKWORK_ASSET_AUDIO_F32).i(int32_t(slot)).i(int32_t(frames * 4))
             .i(1).i(int32_t(frames)).f(float(mSampleRate)).bytes());
    send(osc::Message("/supersonic/piano/wavetable").i(bufnum).bytes());
    return true;
}

// ── The engine's replies ─────────────────────────────────────────────────

void Core::pollEngine() {
    ClockworkClientMessage msgs[64];
    while (true) {
        const uint32_t n = clockwork_client_poll(mClient, msgs, 64);
        for (uint32_t i = 0; i < n; i++) handleReply(msgs[i].bytes, msgs[i].length);
        if (n < 64) break;
    }
}

void Core::handleReply(const uint8_t* p, uint32_t n) {
    osc::Parsed m;
    if (!osc::parse(p, n, m)) return;
    const std::string cmd = !m.args.empty() && m.args[0].type == osc::Value::String ? m.args[0].text : "";
    if (m.address == "/done") {
        if (cmd == "/d_recv" && mDefInFlight) {
            mDefsReady.insert(mDefInFlightName);
            mDefInFlight = false;
            sendNextSynthdef();
        } else if (cmd == "/supersonic/piano/wavetable") {
            mPianoLoaded = true;
            if (m.args.size() > 1) send(osc::Message("/b_free").i(int32_t(m.args[1].integer)).bytes());
            else send(osc::Message("/b_free").i(mNextOwnBuffer - 1).bytes());
        }
    } else if (m.address == "/fail") {
        const std::string why = m.args.size() > 1 ? m.args[1].str() : "";
        if (cmd == "/d_recv" && mDefInFlight) {
            emit(Event::Error, "The synth " + mDefInFlightName + " would not load: " + why);
            mDefsReady.insert(mDefInFlightName);
            mDefInFlight = false;
            sendNextSynthdef();
        } else if (cmd == "/supersonic/piano/wavetable") {
            mPianoLoaded = true;
            emit(Event::Error, "The engine refused the piano's table: :piano will be silent.");
        } else if (cmd != "/b_free") {
            emit(Event::Engine, cmd + ": " + why);
        }
    } else if (m.address == "/clockwork/asset/committed" && !m.args.empty()) {
        const int32_t id = int32_t(m.args[0].integer);
        mBuffersReady.insert(id);
        mBufferAsked.erase(id);
    } else if (m.address == "/clockwork/asset/refused" && !m.args.empty()) {
        const int32_t id = int32_t(m.args[0].integer);
        emit(Event::Error, "The engine refused a sample: " + (m.args.size() > 1 ? m.args[1].str() : std::string()));
        if (auto it = mBufferSlots.find(id); it != mBufferSlots.end()) { clockwork_asset_pool_free(mPool, it->second); mBufferSlots.erase(it); }
        mBuffersReady.insert(id);
        mBufferAsked.erase(id);
    } else if (m.address == "/clockwork/asset/released" && !m.args.empty()) {
        const int32_t id = int32_t(m.args[0].integer);
        if (auto it = mBufferSlots.find(id); it != mBufferSlots.end()) { clockwork_asset_pool_free(mPool, it->second); mBufferSlots.erase(it); }
    }
}

// ── The runtime's records, for the UI (sonic-pi/app/web/web/gui-stream.js) ──

void Core::record(const uint8_t* p, uint32_t n) {
    osc::Parsed m;
    if (!osc::parse(p, n, m) || m.args.size() < 4 || m.address.size() <= 10) return;
    const std::string kind = m.address.substr(10);
    const int uid = int(m.args[0].integer);
    const double time = m.args[1].asNumber() - (mT0 < 0 ? m.args[1].asNumber() : mT0);
    const int job = int(m.args[2].integer);
    const int line = m.args[3].type == osc::Value::Int ? int(m.args[3].integer) : 0;
    const std::vector<osc::Value> f(m.args.begin() + 4, m.args.end());
    auto field = [&](size_t i) -> const osc::Value& { static const osc::Value nil; return i < f.size() ? f[i] : nil; };

    if (kind == "thread_start") {
        mThreadNames[uid] = field(3).type == osc::Value::String ? field(3).text : "";
        return;
    }
    const std::string thread = mThreadNames.count(uid) ? mThreadNames[uid] : "";
    if (kind == "thread_end") { mThreadNames.erase(uid); return; }
    if (kind == "synth") {
        std::string synth = field(2).text;
        if (synth.rfind("sonic-pi-fx_", 0) == 0 || synth.find("mixer") != std::string::npos) return;
        if (synth.rfind("sonic-pi-", 0) == 0) synth = synth.substr(9);
        const osc::Value& args = field(5);
        const osc::Value* buf = optValue(args, "buf");
        std::string text = buf && buf->type == osc::Value::String
            ? "sample :" + stem(buf->text) + ", " + optsText(args, "buf")
            : "synth :" + synth + ", " + optsText(args, nullptr);
        emit(Event::Synth, text, job, time, line, thread);
    } else if (kind == "output" || kind == "log") {
        const std::string text = field(1).str();
        if (kind == "log" && (text.rfind("synth ", 0) == 0 || text.rfind("sample ", 0) == 0)) return;   // the synth record says it
        emit(kind == "output" ? Event::Output : Event::Log, field(1).str(), job, time, line, thread);
    } else if (kind == "error") {
        std::string text = field(1).str();
        if (field(0).type == osc::Value::String && !field(0).text.empty()) text = field(0).text + ": " + text;
        emit(Event::Error, text, job, time, line, thread);
    } else if (kind == "cue") {
        std::string text = field(2).str();
        if (field(3).type == osc::Value::Array && !field(3).items.empty()) text += " " + field(3).str();
        emit(Event::Cue, text, job, time, line, thread);
    }
}

// ── The clock and the audio thread ────────────────────────────────────────

double Core::engineNow() const {
    double ntp;
    int64_t wall;
    uint32_t seq;
    do {
        seq = mClockSeq.load(std::memory_order_acquire);
        ntp = mClockNtp.load(std::memory_order_relaxed);
        wall = mClockWallNs.load(std::memory_order_relaxed);
    } while ((seq & 1) || seq != mClockSeq.load(std::memory_order_acquire));
    // Offline, nothing moves but render(); in real time the clock runs on
    // between audio callbacks (capped, so a stalled device does not race it).
    if (!mThread.joinable()) return ntp;
    return ntp + std::min(0.25, double(steadyNs() - wall) / 1e9);
}

void Core::render(float* const* out, uint32_t channels, uint32_t frames) {
    if (!mEmbed) {
        for (uint32_t c = 0; c < channels; c++) if (out[c]) std::memset(out[c], 0, frames * sizeof(float));
        return;
    }
    SP_ARM_FTZ();
    clockwork_embed_render(mEmbed, out, channels, nullptr, 0, frames);
    // The end of the last block rendered, on the engine's own clock. The block
    // time is an NTP timetag (32.32) in an int64: past 2036 it reads negative
    // signed, so it is read as the unsigned value it is.
    const double ntp = double(uint64_t(clockwork_block_time())) / 4294967296.0 + clockwork_embed_block_size(mEmbed) / mSampleRate;
    const uint32_t seq = mClockSeq.load(std::memory_order_relaxed);
    mClockSeq.store(seq + 1, std::memory_order_release);
    mClockNtp.store(ntp, std::memory_order_relaxed);
    mClockWallNs.store(steadyNs(), std::memory_order_relaxed);
    mClockSeq.store(seq + 2, std::memory_order_release);
}

} // namespace sonicpi
