// SPDX-License-Identifier: AGPL-3.0-or-later
#include "osc.h"

#include <cmath>
#include <cstdio>

namespace osc {

namespace {

struct Reader {
    const uint8_t* p;
    size_t n, at = 0;

    bool u32(uint32_t& v) {
        if (at + 4 > n) return false;
        v = uint32_t(p[at]) << 24 | uint32_t(p[at + 1]) << 16 | uint32_t(p[at + 2]) << 8 | p[at + 3];
        at += 4;
        return true;
    }
    bool str(std::string& s) {
        size_t end = at;
        while (end < n && p[end]) end++;
        if (end >= n) return false;
        s.assign(reinterpret_cast<const char*>(p + at), end - at);
        at = (end + 4) & ~size_t(3);
        return at <= n;
    }
};

bool readValue(Reader& r, const std::string& tags, size_t& t, Value& v) {
    const char c = tags[t++];
    uint32_t a, b;
    switch (c) {
    case 'i': if (!r.u32(a)) return false; v.type = Value::Int; v.integer = int32_t(a); return true;
    case 'h': if (!r.u32(a) || !r.u32(b)) return false; v.type = Value::Int; v.integer = int64_t(uint64_t(a) << 32 | b); return true;
    case 'f': { if (!r.u32(a)) return false; float f; std::memcpy(&f, &a, 4); v.type = Value::Float; v.number = f; return true; }
    case 'd': {
        if (!r.u32(a) || !r.u32(b)) return false;
        const uint64_t bits = uint64_t(a) << 32 | b;
        double d; std::memcpy(&d, &bits, 8);
        v.type = Value::Float; v.number = d; return true;
    }
    case 's': case 'S': v.type = Value::String; return r.str(v.text);
    case 'b': {
        if (!r.u32(a) || r.at + a > r.n) return false;
        v.type = Value::Blob;
        v.text.assign(reinterpret_cast<const char*>(r.p + r.at), a);
        r.at += (a + 3) & ~3u;
        return true;
    }
    case 't': if (!r.u32(a) || !r.u32(b)) return false; v.type = Value::Int; v.integer = int64_t(uint64_t(a) << 32 | b); return true;
    case 'N': v.type = Value::Nil; return true;
    case 'T': v.type = Value::True; return true;
    case 'F': v.type = Value::False; return true;
    case '[':
        v.type = Value::Array;
        while (t < tags.size() && tags[t] != ']') {
            Value item;
            if (!readValue(r, tags, t, item)) return false;
            v.items.push_back(std::move(item));
        }
        if (t < tags.size()) t++;   // the ']'
        return true;
    default: return false;
    }
}

std::string fmtFloat(double d) {
    if (!std::isfinite(d)) return d > 0 ? "Infinity" : d < 0 ? "-Infinity" : "NaN";
    char buf[32];
    std::snprintf(buf, sizeof buf, "%.4g", d);
    std::string s = buf;
    if (s.find_first_of(".e") == std::string::npos) s += ".0";
    return s;
}

} // namespace

std::string Value::str() const {
    switch (type) {
    case Nil: return "nil";
    case True: return "true";
    case False: return "false";
    case Int: return std::to_string(integer);
    case Float: return fmtFloat(number);
    case String: return text;
    case Blob: return "<" + std::to_string(text.size()) + " bytes>";
    case Array: {
        std::string s = "[";
        for (size_t i = 0; i < items.size(); i++) s += (i ? ", " : "") + items[i].str();
        return s + "]";
    }
    }
    return "";
}

bool parse(const uint8_t* p, size_t n, Parsed& out) {
    Reader r{p, n};
    out.args.clear();
    if (!r.str(out.address) || out.address.empty() || out.address[0] != '/') return false;
    std::string tags;
    if (r.at >= n) return true;                 // no type tags: no arguments
    if (!r.str(tags) || tags.empty() || tags[0] != ',') return false;
    size_t t = 1;
    while (t < tags.size()) {
        Value v;
        if (!readValue(r, tags, t, v)) return false;
        out.args.push_back(std::move(v));
    }
    return true;
}

} // namespace osc
