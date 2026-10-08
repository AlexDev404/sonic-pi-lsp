// SPDX-License-Identifier: AGPL-3.0-or-later
#include "audio_output.h"
#include "sonic_pi_core.h"

#include <android/log.h>
#include <dlfcn.h>
#include <unistd.h>
#include <algorithm>
#include <cmath>
#include <chrono>
#include <thread>

#define LOG(...) __android_log_print(ANDROID_LOG_INFO, "SonicPi", __VA_ARGS__)

namespace {

// How much the stream buffers at first, and the most it asks room for. Sonic
// Pi schedules every sound ahead (sched_ahead_time, half a second by default),
// so a few tens of milliseconds more on the way out moves nothing out of
// time; what it buys is room for a block that is slow to render (many synths
// and FX starting at once) without the device running dry.
constexpr double kStartBufferSeconds = 0.040;
constexpr double kCapacitySeconds = 0.200;

// The performance hint API (Android 13, API 33): the callback says how long
// each render took against its burst, and the system keeps the thread on a
// core and at a clock that can do it, rather than finding out from a dropout.
// Looked up at run time, as the app runs from Android 11.
struct HintApi {
    using Manager = void* (*)();
    using Create = void* (*)(void*, const int32_t*, size_t, int64_t);
    using Report = int (*)(void*, int64_t);
    using Close = void (*)(void*);
    Manager getManager = nullptr;
    Create createSession = nullptr;
    Report reportActual = nullptr;
    Close closeSession = nullptr;
    HintApi() {
        void* lib = dlopen("libandroid.so", RTLD_NOW);
        if (!lib) return;
        getManager = reinterpret_cast<Manager>(dlsym(lib, "APerformanceHint_getManager"));
        createSession = reinterpret_cast<Create>(dlsym(lib, "APerformanceHint_createSession"));
        reportActual = reinterpret_cast<Report>(dlsym(lib, "APerformanceHint_reportActualWorkDuration"));
        closeSession = reinterpret_cast<Close>(dlsym(lib, "APerformanceHint_closeSession"));
    }
    bool available() const { return getManager && createSession && reportActual && closeSession; }
};

const HintApi& hints() {
    static const HintApi api;
    return api;
}

int64_t nowNs() {
    return std::chrono::duration_cast<std::chrono::nanoseconds>(std::chrono::steady_clock::now().time_since_epoch()).count();
}

} // namespace

bool AudioOutput::start(int32_t sampleRate, std::string* error) {
    std::lock_guard<std::mutex> lock(mMutex);
    if (mStream.load()) return true;
    mRate = sampleRate;
    AAudioStreamBuilder* builder = nullptr;
    aaudio_result_t rc = AAudio_createStreamBuilder(&builder);
    if (rc != AAUDIO_OK) { if (error) *error = AAudio_convertResultToText(rc); return false; }
    AAudioStreamBuilder_setDirection(builder, AAUDIO_DIRECTION_OUTPUT);
    AAudioStreamBuilder_setPerformanceMode(builder, AAUDIO_PERFORMANCE_MODE_LOW_LATENCY);
    AAudioStreamBuilder_setSharingMode(builder, AAUDIO_SHARING_MODE_SHARED);
    AAudioStreamBuilder_setFormat(builder, AAUDIO_FORMAT_PCM_FLOAT);
    AAudioStreamBuilder_setChannelCount(builder, mChannels);
    AAudioStreamBuilder_setSampleRate(builder, sampleRate);
    AAudioStreamBuilder_setBufferCapacityInFrames(builder, int32_t(sampleRate * kCapacitySeconds));
    AAudioStreamBuilder_setUsage(builder, AAUDIO_USAGE_MEDIA);
    AAudioStreamBuilder_setContentType(builder, AAUDIO_CONTENT_TYPE_MUSIC);
    AAudioStreamBuilder_setDataCallback(builder, onData, this);
    AAudioStreamBuilder_setErrorCallback(builder, onError, this);
    AAudioStream* stream = nullptr;
    rc = AAudioStreamBuilder_openStream(builder, &stream);
    AAudioStreamBuilder_delete(builder);
    if (rc != AAUDIO_OK) { if (error) *error = AAudio_convertResultToText(rc); return false; }

    // Buffered: whole bursts, about kStartBufferSeconds, within what the device gave.
    mBurst = std::max(1, AAudioStream_getFramesPerBurst(stream));
    mCapacity = std::max(mBurst, AAudioStream_getBufferCapacityInFrames(stream));
    const int32_t want = std::max(2, int32_t(std::ceil(sampleRate * kStartBufferSeconds / mBurst))) * mBurst;
    const int32_t got = AAudioStream_setBufferSizeInFrames(stream, std::min(want, mCapacity));
    mBufferMs.store(1000.0 * std::max(got, 0) / AAudioStream_getSampleRate(stream));
    mXRuns.store(0);
    mCallbackTid.store(0);

    rc = AAudioStream_requestStart(stream);
    if (rc != AAUDIO_OK) {
        AAudioStream_close(stream);
        if (error) *error = AAudio_convertResultToText(rc);
        return false;
    }
    mStream.store(stream);
    LOG("audio: %d Hz, burst %d frames, buffer %.0f ms of %.0f ms", AAudioStream_getSampleRate(stream), mBurst,
        mBufferMs.load(), 1000.0 * mCapacity / AAudioStream_getSampleRate(stream));
    openHintSession();
    return true;
}

void AudioOutput::stop() {
    std::lock_guard<std::mutex> lock(mMutex);
    AAudioStream* stream = mStream.exchange(nullptr);
    if (!stream) return;
    AAudioStream_requestStop(stream);
    AAudioStream_close(stream);
    closeHintSession();
}

// The callback's thread is AAudio's: its id is known once it has run. The
// session is made here, off the callback (it is a call into the system).
void AudioOutput::openHintSession() {
    if (!hints().available()) return;
    for (int i = 0; i < 100 && mCallbackTid.load() == 0; i++) std::this_thread::sleep_for(std::chrono::milliseconds(2));
    const int32_t tid = mCallbackTid.load();
    if (tid == 0) return;
    void* manager = hints().getManager();
    if (!manager) return;
    const int64_t target = int64_t(1e9 * mBurst / std::max(1, mRate));
    void* session = hints().createSession(manager, &tid, 1, target);
    mHint.store(session);
    if (session) LOG("audio: performance hints on, %.1f ms a burst", target / 1e6);
}

void AudioOutput::closeHintSession() {
    if (void* session = mHint.exchange(nullptr)) hints().closeSession(session);
}

// An underrun (the device ran dry while a render was slow): a burst more of
// buffering, up to the room the stream has, as Oboe's latency tuner does.
void AudioOutput::growOnUnderrun(AAudioStream* stream) {
    const int32_t xruns = AAudioStream_getXRunCount(stream);
    if (xruns <= mXRuns.load(std::memory_order_relaxed)) return;
    mXRuns.store(xruns, std::memory_order_relaxed);
    const int32_t size = AAudioStream_getBufferSizeInFrames(stream);
    if (size + mBurst <= mCapacity) {
        const int32_t got = AAudioStream_setBufferSizeInFrames(stream, size + mBurst);
        if (got > 0) mBufferMs.store(1000.0 * got / std::max(1, mRate), std::memory_order_relaxed);
    }
}

aaudio_data_callback_result_t AudioOutput::onData(AAudioStream* stream, void* user, void* audio, int32_t frames) {
    auto* self = static_cast<AudioOutput*>(user);
    const int64_t began = nowNs();
    if (self->mCallbackTid.load(std::memory_order_relaxed) == 0) self->mCallbackTid.store(gettid(), std::memory_order_relaxed);
    auto* out = static_cast<float*>(audio);
    // The engine renders planar; AAudio wants interleaved.
    for (int32_t done = 0; done < frames;) {
        const int32_t n = std::min(kChunk, frames - done);
        float* planes[2] = {self->mLeft, self->mRight};
        self->mCore.render(planes, 2, uint32_t(n));
        for (int32_t i = 0; i < n; i++) {
            out[2 * (done + i)] = self->mLeft[i];
            out[2 * (done + i) + 1] = self->mRight[i];
        }
        done += n;
    }
    self->growOnUnderrun(stream);
    if (void* session = self->mHint.load(std::memory_order_relaxed)) hints().reportActual(session, nowNs() - began);
    return AAUDIO_CALLBACK_RESULT_CONTINUE;
}

// A device change (headphones in or out) disconnects the stream: a new one is
// opened on the new device, off the callback's thread as AAudio requires.
void AudioOutput::onError(AAudioStream*, void* user, aaudio_result_t error) {
    if (error != AAUDIO_ERROR_DISCONNECTED) return;
    auto* self = static_cast<AudioOutput*>(user);
    std::thread([self] { self->restart(); }).detach();
}

void AudioOutput::restart() {
    stop();
    std::string why;
    if (!start(mRate, &why)) LOG("audio: could not reopen the stream: %s", why.c_str());
}
