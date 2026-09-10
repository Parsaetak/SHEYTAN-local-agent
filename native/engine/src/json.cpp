// json.cpp — minimal JSON parse/serialize implementation.

#include "json.h"

#include <cstdio>
#include <cstring>

namespace shtn {
namespace json {

Parsed parse_request(const std::string& bytes) {
    Parsed result;
    result.valid = false;
    result.id = 0;

    detail::Scanner scanner(bytes.data(), bytes.size());

    scanner.skip_ws();

    if (!scanner.consume('{')) {
        result.error = "request is not a JSON object";
        return result;
    }

    bool have_id = false;
    bool have_op = false;

    scanner.skip_ws();

    if (scanner.consume('}')) {
        result.error = "request object is empty";
        return result;
    }

    for (;;) {
        scanner.skip_ws();

        std::string key;

        if (!scanner.read_string(key)) {
            result.error = "malformed object key";
            return result;
        }

        scanner.skip_ws();

        if (!scanner.consume(':')) {
            result.error = "expected ':' after key";
            return result;
        }

        scanner.skip_ws();

        if (key == "id") {
            double value = 0;
            if (!scanner.read_number(value)) {
                result.error = "id is not a number";
                return result;
            }
            result.id = static_cast<int64_t>(value);
            have_id = true;
        } else if (key == "op") {
            if (!scanner.read_string(result.op)) {
                result.error = "op is not a string";
                return result;
            }
            have_op = true;
        } else if (key == "payload") {
            // Capture the raw payload member so op handlers can parse
            // their own shape with the same scanner machinery.
            const size_t start = scanner.position();
            if (!scanner.skip_value()) {
                result.error = "malformed value for member 'payload'";
                return result;
            }
            result.payload_raw.assign(bytes.data() + start,
                                      scanner.position() - start);
            result.has_payload = true;
        } else {
            if (!scanner.skip_value()) {
                result.error = "malformed value for member '" + key + "'";
                return result;
            }
        }

        scanner.skip_ws();

        if (scanner.consume(',')) {
            continue;
        }

        scanner.skip_ws();

        if (scanner.consume('}')) {
            break;
        }

        result.error = "expected ',' or '}' in object";
        return result;
    }

    if (!have_id) {
        result.error = "request has no id";
        return result;
    }

    if (!have_op || result.op.empty()) {
        result.error = "request has no op";
        return result;
    }

    // Trailing garbage after the object is rejected (one frame = one
    // value).
    scanner.skip_ws();

    if (!scanner.at_end()) {
        result.error = "trailing data after request object";
        return result;
    }

    result.valid = true;
    return result;
}

bool extract_load_payload(const std::string& bytes, std::string& path,
                          uint32_t& context_length, std::string& error) {
    path.clear();
    context_length = 0;

    detail::Scanner scanner(bytes.data(), bytes.size());

    scanner.skip_ws();

    if (!scanner.consume('{')) {
        error = "payload is not a JSON object";
        return false;
    }

    bool have_path = false;

    scanner.skip_ws();

    if (scanner.consume('}')) {
        error = "payload has no path";
        return false;
    }

    for (;;) {
        scanner.skip_ws();

        std::string key;

        if (!scanner.read_string(key)) {
            error = "malformed payload key";
            return false;
        }

        scanner.skip_ws();

        if (!scanner.consume(':')) {
            error = "expected ':' after payload key";
            return false;
        }

        scanner.skip_ws();

        if (key == "path") {
            if (!scanner.read_string(path)) {
                error = "payload path is not a string";
                return false;
            }
            have_path = true;
        } else if (key == "contextLength") {
            double value = 0;
            if (!scanner.read_number(value) || value < 0 ||
                value > static_cast<double>(UINT32_MAX)) {
                error = "payload contextLength is not a uint32";
                return false;
            }
            context_length = static_cast<uint32_t>(value);
        } else {
            if (!scanner.skip_value()) {
                error = "malformed value for payload member '" + key + "'";
                return false;
            }
        }

        scanner.skip_ws();

        if (scanner.consume(',')) {
            continue;
        }

        scanner.skip_ws();

        if (scanner.consume('}')) {
            break;
        }

        error = "expected ',' or '}' in payload object";
        return false;
    }

    if (!have_path || path.empty()) {
        error = "payload has no path";
        return false;
    }

    return true;
}

std::string quote(const std::string& src) {
    std::string out;
    out.reserve(src.size() + 2);
    out.push_back('"');

    char buf[8];

    for (const char raw : src) {
        const unsigned char c = static_cast<unsigned char>(raw);

        switch (c) {
        case '"': out += "\\\""; break;
        case '\\': out += "\\\\"; break;
        case '\b': out += "\\b"; break;
        case '\f': out += "\\f"; break;
        case '\n': out += "\\n"; break;
        case '\r': out += "\\r"; break;
        case '\t': out += "\\t"; break;
        default:
            if (c < 0x20) {
                std::snprintf(buf, sizeof(buf), "\\u%04x", static_cast<unsigned>(c));
                out += buf;
            } else {
                out.push_back(static_cast<char>(c));
            }
        }
    }

    out.push_back('"');
    return out;
}

} // namespace json
} // namespace shtn
