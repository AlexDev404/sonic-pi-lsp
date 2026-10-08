// SPDX-License-Identifier: AGPL-3.0-or-later
// The device's output, through AAudio: its data callback renders the engine.
#pragma once

#include <aaudio/AAudio.h>
#include <atomic>
#include <cstdint>
#include <mutex>
#include <string>

namespace sonicpi { class Core; }

class AudioOutput {
public:
    explicit AudioOutput(sonicpi::Core& core) : mCore(core) {}
    ~AudioOutput() { stop(); }

    // Opens and starts a float stereo stream at the engine's rate (Android
    // resamples if the device runs at another), with room to absorb a block
    // that is slow to render.
    bool start(int32_t sampleRate, std::string* error);
    void stop();
    bool playing() const { return mStream.load() != nullptr; }

    // Underruns heard, and the buffering now (ms): it grows with each one.
    int32_t xruns() const { return mXRuns.load(); }
    double bufferMs() const { return mBufferMs.load(); }

private:
    static aaudio_data_callback_result_t onData(AAudioStream*, void* user, void* audio, int32_t frames);
    static void onError(AAudioStream*, void* user, aaudio_result_t error);
    void restart();
    void growOnUnderrun(AAudioStream* stream);
    void openHintSession();
    void closeHintSession();

    sonicpi::Core& mCore;
    std::mutex mMutex;
    std::atomic<AAudioStream*> mStream{nullptr};
    int32_t mRate = 0;
    int32_t mChannels = 2;
    int32_t mBurst = 0;
    int32_t mCapacity = 0;
    std::atomic<int32_t> mXRuns{0};
    std::atomic<double> mBufferMs{0};
    // The callback's thread, for the performance hint session (Android 13+).
    std::atomic<int32_t> mCallbackTid{0};
    std::atomic<void*> mHint{nullptr};
    static constexpr int32_t kChunk = 1024;
    float mLeft[kChunk] = {}, mRight[kChunk] = {};
};
