// json.cpp — minimal JSON parse/serialize implementation.

#include "json.h"

#include <cstdio>
#include <cstring>

namespace shtn {
namespace json {
namespace {

// Scanner walks the input with explicit bounds; it never throws and
// never reads past end.
class Scanner {
public:
    Scanner(const char* data, size_t size)
        : data_(data), size_(size), pos_(0) {}

    void skip_ws() {
        while (pos_ < size_) {
            const char c = data_[pos_];
            if (c == ' ' || c == '\t' || c == '\n' || c == '\r') {
                ++pos_;
            } else {
                break;
            }
        }
    }

    bool at_end() const { return pos_ >= size_; }
    char peek() const { return pos_ < size_ ? data_[pos_] : '\0'; }

    bool consume(char c) {
        if (pos_ < size_ && data_[pos_] == c) {
            ++pos_;
            return true;
        }
        return false;
    }

    // read_string parses a JSON string literal (including escapes).
    bool read_string(std::string& out) {
        out.clear();

        if (!consume('"')) {
            return false;
        }

        while (pos_ < size_) {
            const char c = data_[pos_++];

            if (c == '"') {
                return true;
            }

            if (static_cast<unsigned char>(c) < 0x20) {
                return false; // raw control character: invalid JSON
            }

            if (c != '\\') {
                out.push_back(c);
                continue;
            }

            if (pos_ >= size_) {
                return false;
            }

            const char esc = data_[pos_++];

            switch (esc) {
            case '"': out.push_back('"'); break;
            case '\\': out.push_back('\\'); break;
            case '/': out.push_back('/'); break;
            case 'b': out.push_back('\b'); break;
            case 'f': out.push_back('\f'); break;
            case 'n': out.push_back('\n'); break;
            case 'r': out.push_back('\r'); break;
            case 't': out.push_back('\t'); break;
            case 'u': {
                if (pos_ + 4 > size_) {
                    return false;
                }

                unsigned code = 0;
                for (int i = 0; i < 4; ++i) {
                    code <<= 4;
                    const char h = data_[pos_++];
                    if (h >= '0' && h <= '9') {
                        code |= static_cast<unsigned>(h - '0');
                    } else if (h >= 'a' && h <= 'f') {
                        code |= static_cast<unsigned>(h - 'a' + 10);
                    } else if (h >= 'A' && h <= 'F') {
                        code |= static_cast<unsigned>(h - 'A' + 10);
                    } else {
                        return false;
                    }
                }

                // Encode BMP scalar as UTF-8 (surrogates pass through
                // paired; a lone surrogate is tolerated as replacement).
                if (code >= 0xD800u && code <= 0xDFFFu) {
                    out.push_back('\xEF'); out.push_back('\xBF'); out.push_back('\xBD');
                } else if (code < 0x80u) {
                    out.push_back(static_cast<char>(code));
                } else if (code < 0x800u) {
                    out.push_back(static_cast<char>(0xC0u | (code >> 6)));
                    out.push_back(static_cast<char>(0x80u | (code & 0x3Fu)));
                } else {
                    out.push_back(static_cast<char>(0xE0u | (code >> 12)));
                    out.push_back(static_cast<char>(0x80u | ((code >> 6) & 0x3Fu)));
                    out.push_back(static_cast<char>(0x80u | (code & 0x3Fu)));
                }
                break;
            }
            default:
                return false; // invalid escape
            }
        }

        return false; // unterminated string
    }

    // skip_value skips any JSON value (object/array/string/number/bool/
    // null) without retaining it. Objects are key:value sequences; arrays
    // are bare value sequences — both handled recursively.
    bool skip_value() {
        skip_ws();

        if (at_end()) {
            return false;
        }

        const char c = peek();

        if (c == '"') {
            std::string sink;
            return read_string(sink);
        }

        if (c == '{' || c == '[') {
            const char open = c;
            const char close = (open == '{') ? '}' : ']';
            const bool is_object = (open == '{');

            if (!consume(open)) {
                return false;
            }

            skip_ws();

            if (consume(close)) {
                return true; // empty container
            }

            for (;;) {
                if (is_object) {
                    // object member: string key, ':', value
                    std::string key;

                    skip_ws();

                    if (!read_string(key)) {
                        return false;
                    }

                    skip_ws();

                    if (!consume(':')) {
                        return false;
                    }
                }

                if (!skip_value()) {
                    return false;
                }

                skip_ws();

                if (consume(',')) {
                    continue;
                }

                skip_ws();

                if (consume(close)) {
                    return true;
                }

                return false;
            }
        }

        // number / true / false / null: consume a run of legal bytes.
        size_t start = pos_;

        while (pos_ < size_) {
            const char b = data_[pos_];

            const bool legal =
                (b >= '0' && b <= '9') || b == '-' || b == '+' || b == '.' ||
                b == 'e' || b == 'E' || b == 't' || b == 'r' || b == 'u' ||
                b == 'f' || b == 'a' || b == 'l' || b == 's' || b == 'n';

            if (!legal) {
                break;
            }
            ++pos_;
        }

        return pos_ > start;
    }

    // read_number consumes a numeric literal into out.
    bool read_number(double& out) {
        skip_ws();

        const size_t start = pos_;

        while (pos_ < size_) {
            const char b = data_[pos_];
            const bool legal =
                (b >= '0' && b <= '9') || b == '-' || b == '+' || b == '.' ||
                b == 'e' || b == 'E';
            if (!legal) {
                break;
            }
            ++pos_;
        }

        if (pos_ == start) {
            return false;
        }

        out = std::strtod(std::string(data_ + start, pos_ - start).c_str(), nullptr);
        return true;
    }

private:
    const char* data_;
    size_t size_;
    size_t pos_;
};

} // namespace

Parsed parse_request(const std::string& bytes) {
    Parsed result;
    result.valid = false;
    result.id = 0;

    Scanner scanner(bytes.data(), bytes.size());

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
