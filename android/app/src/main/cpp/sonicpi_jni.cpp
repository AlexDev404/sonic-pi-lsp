// SPDX-License-Identifier: AGPL-3.0-or-later
// The JNI the app's Kotlin calls (engine/NativeBridge.kt). One engine per
// process. Events from the core are queued here and polled by the app, so no
// native thread ever calls into the JVM.
#include "audio_output.h"
#include "sonic_pi_core.h"

#include <jni.h>
#include <deque>
#include <memory>
#include <mutex>
#include <string>

namespace {

std::unique_ptr<sonicpi::Core> gCore;
std::unique_ptr<AudioOutput> gAudio;
std::mutex gEventsMutex;
std::deque<sonicpi::Event> gEvents;
constexpr size_t kMaxQueued = 2000;

std::string str(JNIEnv* env, jstring s) {
    if (!s) return {};
    const char* chars = env->GetStringUTFChars(s, nullptr);
    std::string out(chars);
    env->ReleaseStringUTFChars(s, chars);
    return out;
}

// Modified UTF-8 is what NewStringUTF takes: anything else (a NUL, an
// invalid byte) is replaced so a stray byte in a log line cannot crash the app.
jstring jstr(JNIEnv* env, const std::string& s) {
    std::string clean;
    clean.reserve(s.size());
    for (size_t i = 0; i < s.size();) {
        const unsigned char c = static_cast<unsigned char>(s[i]);
        size_t len = c < 0x80 ? 1 : (c >> 5) == 0x6 ? 2 : (c >> 4) == 0xE ? 3 : (c >> 3) == 0x1E ? 4 : 0;
        bool ok = len > 0 && len < 4 && i + len <= s.size() && c != 0;
        for (size_t k = 1; ok && k < len; k++) ok = (static_cast<unsigned char>(s[i + k]) >> 6) == 0x2;
        if (ok) { clean.append(s, i, len); i += len; }
        else { clean += "\xEF\xBF\xBD"; i += len ? std::min(len, s.size() - i) : 1; }
    }
    return env->NewStringUTF(clean.c_str());
}

} // namespace

#define JNI(type, name) extern "C" JNIEXPORT type JNICALL Java_io_github_alexdev404_sonicpi_engine_NativeBridge_##name

// "" when ready, else what went wrong.
JNI(jstring, nativeBoot)(JNIEnv* env, jobject, jstring assets, jint sampleRate) {
    if (gCore) return jstr(env, "");
    auto core = std::make_unique<sonicpi::Core>([](const sonicpi::Event& e) {
        std::lock_guard<std::mutex> lock(gEventsMutex);
        if (gEvents.size() >= kMaxQueued) gEvents.pop_front();
        gEvents.push_back(e);
    });
    sonicpi::Config config;
    config.assets = str(env, assets);
    config.sampleRate = sampleRate > 0 ? sampleRate : 48000;
    std::string err;
    if (!core->boot(config, &err)) return jstr(env, err.empty() ? "the engine did not start" : err);
    core->start();
    gCore = std::move(core);
    gAudio = std::make_unique<AudioOutput>(*gCore);
    return jstr(env, "");
}

JNI(jstring, nativeStartAudio)(JNIEnv* env, jobject) {
    if (!gCore || !gAudio) return jstr(env, "not booted");
    std::string err;
    if (!gAudio->start(int32_t(gCore->sampleRate()), &err)) return jstr(env, err);
    return jstr(env, "");
}

JNI(void, nativeStopAudio)(JNIEnv*, jobject) {
    if (gAudio) gAudio->stop();
}

JNI(void, nativeRun)(JNIEnv* env, jobject, jstring code) {
    if (gCore) gCore->run(str(env, code));
}

JNI(void, nativeStop)(JNIEnv*, jobject) {
    if (gCore) gCore->stop();
}

JNI(jboolean, nativeIsRunning)(JNIEnv*, jobject) {
    return gCore && gCore->running() ? JNI_TRUE : JNI_FALSE;
}

// Every event queued since the last poll, each as
// kind \x1f job \x1f time \x1f line \x1f thread \x1f text.
JNI(jobjectArray, nativePollEvents)(JNIEnv* env, jobject) {
    std::deque<sonicpi::Event> events;
    {
        std::lock_guard<std::mutex> lock(gEventsMutex);
        events.swap(gEvents);
    }
    jclass stringClass = env->FindClass("java/lang/String");
    jobjectArray out = env->NewObjectArray(jsize(events.size()), stringClass, nullptr);
    for (size_t i = 0; i < events.size(); i++) {
        const auto& e = events[i];
        const std::string row = std::to_string(int(e.kind)) + '\x1f' + std::to_string(e.job) + '\x1f' +
                                std::to_string(e.time) + '\x1f' + std::to_string(e.line) + '\x1f' + e.thread + '\x1f' + e.text;
        jstring s = jstr(env, row);
        env->SetObjectArrayElement(out, jsize(i), s);
        env->DeleteLocalRef(s);
    }
    return out;
}
