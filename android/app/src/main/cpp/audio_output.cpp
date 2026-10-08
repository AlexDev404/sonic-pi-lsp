// SPDX-License-Identifier: AGPL-3.0-or-later
#include "audio_output.h"
#include "sonic_pi_core.h"

#include <android/log.h>
#include <algorithm>
#include <thread>

#define LOG(...) __android_log_print(ANDROID_LOG_INFO, "SonicPi", __VA_ARGS__)

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
    AAudioStreamBuilder_setUsage(builder, AAUDIO_USAGE_MEDIA);
    AAudioStreamBuilder_setContentType(builder, AAUDIO_CONTENT_TYPE_MUSIC);
    AAudioStreamBuilder_setDataCallback(builder, onData, this);
    AAudioStreamBuilder_setErrorCallback(builder, onError, this);
    AAudioStream* stream = nullptr;
    rc = AAudioStreamBuilder_openStream(builder, &stream);
    AAudioStreamBuilder_delete(builder);
    if (rc != AAUDIO_OK) { if (error) *error = AAudio_convertResultToText(rc); return false; }
    // Two bursts of buffering: low latency, with room for a late callback.
    AAudioStream_setBufferSizeInFrames(stream, AAudioStream_getFramesPerBurst(stream) * 2);
    rc = AAudioStream_requestStart(stream);
    if (rc != AAUDIO_OK) {
        AAudioStream_close(stream);
        if (error) *error = AAudio_convertResultToText(rc);
        return false;
    }
    mStream.store(stream);
    LOG("audio: %d Hz, burst %d frames", AAudioStream_getSampleRate(stream), AAudioStream_getFramesPerBurst(stream));
    return true;
}

void AudioOutput::stop() {
    std::lock_guard<std::mutex> lock(mMutex);
    AAudioStream* stream = mStream.exchange(nullptr);
    if (!stream) return;
    AAudioStream_requestStop(stream);
    AAudioStream_close(stream);
}

aaudio_data_callback_result_t AudioOutput::onData(AAudioStream*, void* user, void* audio, int32_t frames) {
    auto* self = static_cast<AudioOutput*>(user);
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
