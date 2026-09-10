// json.h — minimal JSON parsing/serialization for the IPC host.
//
// The wire protocol needs exactly two things:
//   1. parse a request object: {"id": <int>, "op": "<string>", ...}
//      (tolerantly — extra members, nested values and malformed input
//      must never crash the host);
//   2. serialize response objects with correctly escaped strings.
//
// No third-party dependency, no allocations beyond the result. This is
// deliberately NOT a general-purpose JSON library.

#ifndef SHTN_JSON_H
#define SHTN_JSON_H

#include <cstdint>
#include <string>

namespace shtn {
namespace json {

// Parsed is the extracted request envelope.
struct Parsed {
    bool valid;       // syntactically a JSON object with id/op readable
    int64_t id;
    std::string op;
    std::string error; // human-readable parse failure detail
};

// Parse extracts "id" and "op" from a top-level JSON object. Unknown or
// malformed members are ignored unless they break structure; structural
// breakage is reported via valid=false + error.
Parsed parse_request(const std::string& bytes);

// Escape returns src quoted and escaped as a JSON string literal
// (control characters, quotes, backslashes, and invalid UTF-8 bytes).
std::string quote(const std::string& src);

} // namespace json
} // namespace shtn

#endif /* SHTN_JSON_H */
