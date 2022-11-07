.PHONY: all test coverage clean serve

OBJ := kuuf-library
BUILD_DIR := build
COVERAGE_OBJ := coverage.out
SRC := $(shell find internal/ *.go go.mod go.sum)
SERVE_ARGS := $(shell grep -s -v "^\#" .env)
GO_ARGS :=
GO := $(GO_ARGS) go

all: $(BUILD_DIR)/$(OBJ)

test: $(BUILD_DIR)/$(COVERAGE_OBJ)

coverage: $(BUILD_DIR)/$(COVERAGE_OBJ)
	$(GO) go tool cover -html=$<

clean:
	rm -rf $(BUILD_DIR)

serve: $(BUILD_DIR)/$(OBJ)
	$(SERVE_ARGS) $<

$(BUILD_DIR):
	mkdir -p $@

$(BUILD_DIR)/$(OBJ): $(BUILD_DIR)/$(COVERAGE_OBJ) | $(BUILD_DIR)
	$(GO) build -o $@

$(BUILD_DIR)/$(COVERAGE_OBJ): $(SRC) | $(BUILD_DIR)
	$(GO) test ./... -coverprofile=$@