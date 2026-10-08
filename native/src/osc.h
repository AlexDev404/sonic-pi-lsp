// SPDX-License-Identifier: AGPL-3.0-or-later
// A small OSC writer and reader: enough for the messages the host sends the
// engine (/d_recv, /b_free, /clockwork/asset/commit ...) and for reading what
// comes back from the engine and from the runtime's GUI stream (sp_host.c).
#pragma once

#include <cstdint>
#include <cstring>
#include <string>
#include <vector>

namespace osc {

inline void putU32(std::vector<uint8_t>& b, uint32_t v) {
    b.push_back(uint8_t(v >> 24)); b.push_back(uint8_t(v >> 16));
    b.push_back(uint8_t(v >> 8));  b.push_back(uint8_t(v));
}

inline void putPadded(std::vector<uint8_t>& b, const void* p, size_t n) {
    const auto* s = static_cast<const uint8_t*>(p);
    b.insert(b.end(), s, s + n);
    b.insert(b.end(), 4 - n % 4, 0);   // a string ends in one to four NULs
}

// One message, built argument by argument.
class Message {
public:
    explicit Message(std::string address) : mAddress(std::move(address)) {}
    Message& i(int32_t v) { mTags += 'i'; putU32(mData, uint32_t(v)); return *this; }
    Message& f(float v) { uint32_t u; std::memcpy(&u, &v, 4); mTags += 'f'; putU32(mData, u); return *this; }
    Message& s(const std::string& v) { mTags += 's'; putPadded(mData, v.data(), v.size()); return *this; }
    Message& b(const uint8_t* p, size_t n) {
        mTags += 'b';
        putU32(mData, uint32_t(n));
        mData.insert(mData.end(), p, p + n);
        mData.insert(mData.end(), (4 - n % 4) % 4, 0);
        return *this;
    }
    std::vector<uint8_t> bytes() const {
        std::vector<uint8_t> out;
        putPadded(out, mAddress.data(), mAddress.size());
        putPadded(out, mTags.data(), mTags.size());
        out.insert(out.end(), mData.begin(), mData.end());
        return out;
    }
private:
    std::string mAddress;
    std::string mTags = ",";
    std::vector<uint8_t> mData;
};

// A value read back: what an OSC argument can be, arrays included.
struct Value {
    enum Type { Nil, True, False, Int, Float, String, Blob, Array } type = Nil;
    int64_t integer = 0;
    double number = 0;
    std::string text;              // String, or a Blob's bytes
    std::vector<Value> items;      // Array

    bool isNumber() const { return type == Int || type == Float; }
    double asNumber() const { return type == Int ? double(integer) : number; }
    // How Sonic Pi prints it: 60, 0.5, "text", :sym (the stream sends symbols as strings), [..]
    std::string str() const;
};

struct Parsed {
    std::string address;
    std::vector<Value> args;
};

// Parses one OSC message. False when the bytes are not one.
bool parse(const uint8_t* p, size_t n, Parsed& out);

} // namespace osc
