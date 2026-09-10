cmake_minimum_required(VERSION 3.16)

project(shtn_native_engine
    VERSION 1.1.5
    DESCRIPTION "SHEYTAN Native AI Engine — C++ engine core and IPC host (Phase 1 skeleton)"
    LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

option(SHTN_BUILD_TESTS "Build the native engine test binaries" ON)
option(SHTN_BUILD_SHARED "Build libshtn_engine as a shared library instead of static" OFF)

# ---------------------------------------------------------------------------
# Engine core library: the narrow C ABI every consumer links against.
# ---------------------------------------------------------------------------
if(SHTN_BUILD_SHARED)
    add_library(shtn_engine SHARED
        src/engine.cpp
        src/hardware.cpp)
else()
    add_library(shtn_engine STATIC
        src/engine.cpp
        src/hardware.cpp)
endif()

target_include_directories(shtn_engine PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<INSTALL_INTERFACE:include>)

# The engine core is dependency-free C++17 by design (no JSON, no network,
# no third-party libraries) so it builds anywhere, including CI cross-builds.
target_compile_features(shtn_engine PUBLIC cxx_std_17)

if(MSVC)
    target_compile_options(shtn_engine PRIVATE /W4)
else()
    target_compile_options(shtn_engine PRIVATE -Wall -Wextra -Wpedantic)
endif()

# ---------------------------------------------------------------------------
# Protocol helpers (framing + minimal JSON) — host-side only.
# ---------------------------------------------------------------------------
add_library(shtn_protocol STATIC
    src/protocol.cpp
    src/json.cpp)
target_include_directories(shtn_protocol PUBLIC
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
    $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/src>)
target_compile_features(shtn_protocol PUBLIC cxx_std_17)

# ---------------------------------------------------------------------------
# shtn-engine-host: the supervised subprocess the Go core talks to.
# Reads length-prefixed JSON requests on stdin, writes responses on stdout.
# ---------------------------------------------------------------------------
add_executable(shtn-engine-host src/host_main.cpp)
target_link_libraries(shtn-engine-host PRIVATE shtn_engine shtn_protocol)
if(WIN32)
    # Binary-safe stdio on Windows.
    target_compile_definitions(shtn-engine-host PRIVATE SHTN_WINDOWS_HOST=1)
endif()

# ---------------------------------------------------------------------------
# Tests: dependency-free assert-based binaries registered with CTest.
# ---------------------------------------------------------------------------
if(SHTN_BUILD_TESTS)
    enable_testing()

    add_executable(test_engine tests/test_engine.cpp)
    target_link_libraries(test_engine PRIVATE shtn_engine)
    add_test(NAME engine COMMAND test_engine)

    add_executable(test_protocol tests/test_protocol.cpp)
    target_link_libraries(test_protocol PRIVATE shtn_protocol)
    add_test(NAME protocol COMMAND test_protocol)

    add_executable(test_host tests/test_host.cpp)
    target_link_libraries(test_host PRIVATE shtn_engine shtn_protocol)
    add_test(NAME host COMMAND test_host)
endif()
