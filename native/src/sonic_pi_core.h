// SPDX-License-Identifier: AGPL-3.0-or-later
// Sonic Pi in a process: the language runtime (mruby, sonic-pi/app/web/runtime)
// and the engine (SuperSonic: scsynth on clockwork) side by side, with the host
// between them that the web app's page is (sonic-pi/app/web/web/live-core.js).
//
// The host owns the audio callback: it calls render() for every buffer the
// device asks for, and the engine's clock is the frames rendered. A worker
// thread ticks the runtime against that clock, loads what each sound needs
// (synthdefs, samples) before handing its bundle to the engine, and turns the
// runtime's records into Events for a UI.
//
//   Core core(listener);
//   core.boot(config, &err);       // the engine and the runtime, assets described
//   core.start();                  // the worker thread
//   ... audio thread: core.render(out, 2, frames) ...
//   core.run(code); core.stop();
//
// Offline (tests, a render to a file): leave start() out and alternate
// pump() and render() on one thread; loads then happen inside pump().
#pragma once

#include <atomic>
#include <condition_variable>
#include <cstdint>
#include <deque>
#include <functional>
#include <map>
#include <memory>
#include <mutex>
#include <set>
#include <string>
#include <thread>
#include <vector>

struct ClockworkEmbed;
struct ClockworkClient;
struct ClockworkAssetPool;

namespace sonicpi {

struct Event {
    enum Kind {
        Output,   // puts
        Log,      // the runtime's own words
        Error,    // a program's error, or a load that failed
        Cue,      // a cue, set or sync arrival
        Synth,    // a synth or sample started
        State,    // text "running" or "stopped"
        Engine,   // what the host or engine says (booted, a failed load)
    };
    Kind kind = Log;
    int job = 0;            // which Run it belongs to (0: none)
    double time = 0;        // seconds since the session's first Run
    int line = 0;           // the program's line, 0 for none
    std::string thread;     // the thread's name, if it has one (a live_loop's)
    std::string text;
};

struct Config {
    std::string assets;                 // synthdefs/, samples/, buffers/, piano_wavetable.dat
    double sampleRate = 48000;
    uint32_t blockSize = 128;           // the engine's block; the device may ask for any size
    uint32_t outputChannels = 2;
    uint32_t maxRenderFrames = 4096;    // the largest buffer render() is asked for
    uint32_t inboxMB = 160;             // decoded samples live here (reserved lazily)
    uint32_t realTimeMemoryKB = 65536;  // scsynth's pool, as the web app asks for
};

class Core {
public:
    using Listener = std::function<void(const Event&)>;
    explicit Core(Listener listener);
    ~Core();
    Core(const Core&) = delete;
    Core& operator=(const Core&) = delete;

    // The engine attached (no device: render() drives it) and the runtime
    // booted with its random tables and the built-in samples described.
    bool boot(const Config& config, std::string* error);

    // Real time: a worker thread ticks the runtime and loads assets.
    void start();
    void shutdown();

    // Thread-safe: queued for the worker (or the next pump()).
    void run(const std::string& code);
    void stop();

    // The audio thread. Never blocks or allocates.
    void render(float* const* out, uint32_t channels, uint32_t frames);

    // Offline: one turn of the worker loop, loads done synchronously.
    void pump();

    // The engine's clock, NTP seconds: the end of the last block rendered,
    // moved on by the wall time since.
    double engineNow() const;
    double sampleRate() const { return mSampleRate; }
    bool running() const { return mSessionRunning.load(); }
    bool booted() const { return mEmbed != nullptr; }

private:
    struct Sound { std::vector<uint8_t> bundle; std::string synthdef; int32_t buffer; };
    struct Command { enum { Run, Stop } what; std::string code; };

    void loop();
    void turn(bool synchronousLoads);
    void handleCommands();
    void tick();
    void drain();
    void sendSound(Sound& s, double shift);
    bool ready(const Sound& s) const;
    void pollEngine();
    void handleReply(const uint8_t* p, uint32_t n);
    void record(const uint8_t* p, uint32_t n);
    void emit(Event::Kind kind, std::string text, int job = 0, double time = 0, int line = 0, std::string thread = {});

    // Loads. The worker asks; the engine answers (pollEngine).
    void requestSynthdef(const std::string& name);
    void requestSample(int32_t bufnum, const std::string& file);
    void freeSample(int32_t bufnum);
    void processLoads();
    void sendNextSynthdef();
    bool loadSampleNow(int32_t bufnum, const std::string& path);
    bool loadPianoTable();
    std::string resolveSample(const std::string& file) const;
    bool send(const std::vector<uint8_t>& osc);

    Listener mListener;
    Config mConfig;
    ClockworkEmbed* mEmbed = nullptr;
    ClockworkClient* mClient = nullptr;
    ClockworkAssetPool* mPool = nullptr;
    uint8_t* mInbox = nullptr;
    size_t mInboxBytes = 0;
    double mSampleRate = 0;

    // The clock, published by the audio thread (a seqlock: mClockSeq is odd while it writes).
    std::atomic<uint32_t> mClockSeq{0};
    std::atomic<double> mClockNtp{0};
    std::atomic<int64_t> mClockWallNs{0};
    std::atomic<bool> mClearSchedule{false};

    // The worker.
    std::thread mThread;
    std::mutex mMutex;
    std::condition_variable mWake;
    std::deque<Command> mCommands;
    bool mQuit = false;
    std::atomic<bool> mSessionRunning{false};
    double mNextWake = -1;          // when the runtime wants ticking again, -1 for never
    double mT0 = -1;                // the session's first Run, for Event::time

    // What the runtime has numbered, and what the engine holds.
    std::vector<std::string> mSynthdefNames;          // by the runtime's number
    std::set<std::string> mDefsReady, mDefsAsked;
    std::deque<std::string> mDefQueue;                // to /d_recv, one at a time
    bool mDefInFlight = false;
    std::string mDefInFlightName;
    std::set<int32_t> mBuffersReady;
    std::map<int32_t, std::string> mBufferAsked;      // bufnum → file, not yet committed
    std::map<int32_t, uint32_t> mBufferSlots;         // bufnum → inbox offset, held until released
    std::deque<std::pair<int32_t, std::string>> mSampleQueue;
    std::deque<Sound> mWaiting;                       // sounds held back for a load, in order
    bool mPianoLoaded = false;
    int32_t mNextOwnBuffer = 4000;                    // the host's own buffers (the piano table)
    std::map<int, std::string> mThreadNames;          // uid → name
};

} // namespace sonicpi
