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

    // Opens and starts a low-latency float stereo stream at the engine's rate.
    // Android resamples if the device runs at another.
    bool start(int32_t sampleRate, std::string* error);
    void stop();
    bool playing() const { return mStream.load() != nullptr; }

private:
    static aaudio_data_callback_result_t onData(AAudioStream*, void* user, void* audio, int32_t frames);
    static void onError(AAudioStream*, void* user, aaudio_result_t error);
    void restart();

    sonicpi::Core& mCore;
    std::mutex mMutex;
    std::atomic<AAudioStream*> mStream{nullptr};
    int32_t mRate = 0;
    int32_t mChannels = 2;
    static constexpr int32_t kChunk = 1024;
    float mLeft[kChunk] = {}, mRight[kChunk] = {};
};
