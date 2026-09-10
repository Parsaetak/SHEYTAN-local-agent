# SHEYTAN Native AI Engine — C++ skeleton (v1.1.5Z Phase 1)
#
# Build with CMake (preferred):
#
#   cmake -S native/engine -B native/engine/build
#   cmake --build native/engine/build
#   ctest --test-dir native/engine/build
#
# Or with plain make (no CMake required):
#
#   make -C native/engine
#   make -C native/engine test
#
# Artifacts:
#   shtn-engine-host        the supervised IPC host executable
#   libshtn_engine.a        the engine core with the C ABI
#
# Install location for runtime use: {DataDir}/bin/shtn-engine-host(.exe)
# (see internal/native/engine.DefaultHostPath in the Go core).

CXX ?= g++
CXXFLAGS ?= -std=c++17 -Wall -Wextra -Wpedantic -O2
BUILD := build

.PHONY: all clean test host library

all: host library

library: $(BUILD)/libshtn_engine.a

$(BUILD)/libshtn_engine.a: src/engine.cpp src/hardware.cpp include/shtn/engine.h include/shtn/types.h include/shtn/version.h
	@mkdir -p $(BUILD)
	$(CXX) $(CXXFLAGS) -Iinclude -c src/engine.cpp -o $(BUILD)/engine.o
	$(CXX) $(CXXFLAGS) -Iinclude -c src/hardware.cpp -o $(BUILD)/hardware.o
	ar rcs $@ $(BUILD)/engine.o $(BUILD)/hardware.o

$(BUILD)/libshtn_protocol.a: src/protocol.cpp src/json.cpp src/protocol.h src/json.h
	@mkdir -p $(BUILD)
	$(CXX) $(CXXFLAGS) -Iinclude -Isrc -c src/protocol.cpp -o $(BUILD)/protocol.o
	$(CXX) $(CXXFLAGS) -Iinclude -Isrc -c src/json.cpp -o $(BUILD)/json.o
	ar rcs $@ $(BUILD)/protocol.o $(BUILD)/json.o

host: $(BUILD)/shtn-engine-host

$(BUILD)/shtn-engine-host: src/host_main.cpp $(BUILD)/libshtn_engine.a $(BUILD)/libshtn_protocol.a
	@mkdir -p $(BUILD)
	$(CXX) $(CXXFLAGS) -Iinclude -Isrc src/host_main.cpp $(BUILD)/libshtn_engine.a $(BUILD)/libshtn_protocol.a -o $@

test: $(BUILD)/test_engine $(BUILD)/test_protocol $(BUILD)/test_host
	$(BUILD)/test_engine
	$(BUILD)/test_protocol
	$(BUILD)/test_host

$(BUILD)/test_engine: tests/test_engine.cpp $(BUILD)/libshtn_engine.a
	@mkdir -p $(BUILD)
	$(CXX) $(CXXFLAGS) -Iinclude tests/test_engine.cpp $(BUILD)/libshtn_engine.a -o $@

$(BUILD)/test_protocol: tests/test_protocol.cpp $(BUILD)/libshtn_protocol.a
	@mkdir -p $(BUILD)
	$(CXX) $(CXXFLAGS) -Iinclude -Isrc tests/test_protocol.cpp $(BUILD)/libshtn_protocol.a -o $@

$(BUILD)/test_host: tests/test_host.cpp $(BUILD)/libshtn_engine.a $(BUILD)/libshtn_protocol.a
	@mkdir -p $(BUILD)
	$(CXX) $(CXXFLAGS) -Iinclude -Isrc tests/test_host.cpp $(BUILD)/libshtn_engine.a $(BUILD)/libshtn_protocol.a -o $@

clean:
	rm -rf $(BUILD)
