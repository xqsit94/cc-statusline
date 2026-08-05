# cc-statusline
#
# The targets that matter: `make check` before committing, `make gate` before
# signing off on anything visual, `make install` to use it yourself.

BIN     := cc-statusline
OUT     := bin/$(BIN)
PREFIX  ?= $(HOME)/.local/bin
PKG     := github.com/xqsit94/cc-statusline

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Mirrors .goreleaser.yaml. A locally built binary reports the same shape of
# version string as a released one, so a bug report is readable either way.
LDFLAGS := -s -w \
	-X $(PKG)/cmd.version=$(VERSION) \
	-X $(PKG)/cmd.commit=$(COMMIT) \
	-X $(PKG)/cmd.date=$(DATE)

.PHONY: all build test vet fmt fmt-check check install uninstall gate probe bench clean release-dry

all: check build

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(OUT) .
	@echo "built $(OUT) ($(VERSION))"

test:
	go test ./... -count=1

vet:
	go vet ./...

fmt:
	gofmt -w .

# Fails rather than fixes, for CI.
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; echo "$$unformatted"; exit 1; \
	fi

check: fmt-check vet test

install: build
	@mkdir -p $(PREFIX)
	install -m 0755 $(OUT) $(PREFIX)/$(BIN)
	@echo "installed $(PREFIX)/$(BIN)"
	@echo "next: $(BIN) init"

uninstall:
	rm -f $(PREFIX)/$(BIN)

# PRD §9.4's manual visual gate. No test can substitute for it — goldens measure
# with the same go-runewidth the renderer uses, so they prove self-consistency
# and never that your terminal agrees. See docs/M4-visual-gate.md.
gate: build
	$(OUT) preview --matrix

# C-7: install this as the statusLine command and read width_reserve off
# Claude Code's own rendering. docs/M4-visual-gate.md §4.
probe: build
	@echo '{}' | $(OUT) preview --probe

bench:
	go test ./internal/line/ -run '^$$' -bench . -benchmem

# Builds every release artefact locally without publishing anything.
release-dry:
	goreleaser release --snapshot --clean --skip=publish

clean:
	rm -rf bin dist
