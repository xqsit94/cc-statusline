BIN     := cc-statusline
OUT     := bin/$(BIN)
PREFIX  ?= $(HOME)/.local/bin
PKG     := github.com/xqsit94/cc-statusline

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/cmd.version=$(VERSION) \
	-X $(PKG)/cmd.commit=$(COMMIT) \
	-X $(PKG)/cmd.date=$(DATE)

.PHONY: all build test vet fmt fmt-check check install uninstall gate gate-check probe bench golden fuzz p99 clean release-dry

FUZZTIME ?= 60s

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

gate: build
	$(OUT) preview --matrix

gate-check:
	go test ./cmd/ -run TestGate -count=1 -v

probe: build
	@echo '{}' | $(OUT) preview --probe

bench:
	@out=$$(go test ./internal/line/ -run '^$$' -bench . -benchmem); \
	echo "$$out"; \
	case "$$out" in \
		*Benchmark*) ;; \
		*) echo "make: no benchmark ran — -bench matched nothing, and go test called that PASS"; exit 1 ;; \
	esac

golden:
	go test ./internal/line/ -run 'TestPlainGoldens|TestStyledGoldens' -update -count=1
	@git --no-pager diff --stat -- internal/line/testdata || true

fuzz:
	go test ./cmd/ -run '^$$' -fuzz FuzzRender -fuzztime $(FUZZTIME)

p99:
	@CC_STATUSLINE_P99_BIG_REPO=1 go test ./cmd/ -run 'TestRenderProcessBudget|TestFileCountDoesNotMatter' -count=1 -v -timeout 300s
	@command -v hyperfine >/dev/null 2>&1 || { \
		echo; \
		echo "hyperfine is not installed — the Go harness above is the whole measurement."; \
		echo "§8.1 names 'hyperfine --shell=none' for its warm-up and outlier handling:"; \
		echo "  cargo install hyperfine   |   brew install hyperfine   |   pacman -S hyperfine"; \
		exit 0; \
	}; \
	$(MAKE) --no-print-directory build; \
	echo; \
	hyperfine --shell=none --warmup 20 --runs 200 \
		--input internal/refstate/payloads/danger-92.json \
		'$(OUT) render'

release-dry:
	goreleaser release --snapshot --clean --skip=publish

clean:
	rm -rf bin dist
