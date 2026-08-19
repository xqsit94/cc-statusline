#!/bin/sh

set -eu

REPO="${REPO:-xqsit94/cc-statusline}"
BIN="cc-statusline"

say()  { printf '%s\n' "$*"; }
warn() { printf '%s\n' "$*" >&2; }
die()  { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is required but was not found"
}

if [ -z "${PREFIX:-}" ]; then
	if [ -n "${XDG_BIN_HOME:-}" ]; then
		PREFIX="$XDG_BIN_HOME"
	else
		PREFIX="$HOME/.local/bin"
	fi
fi

os=$(uname -s)
arch=$(uname -m)

case "$os" in
	Linux)  OS=Linux ;;
	Darwin) OS=Darwin ;;
	MINGW*|MSYS*|CYGWIN*) die "Windows is not supported yet (PRD §13). Build from source if you want to try it." ;;
	*) die "unsupported operating system: $os" ;;
esac

case "$arch" in
	x86_64|amd64)  ARCH=x86_64 ;;
	arm64|aarch64) ARCH=arm64 ;;
	*) die "unsupported architecture: $arch" ;;
esac

need uname
need tar
if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL "$1" -o "$2"; }
	fetch_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -qO "$2" "$1"; }
	fetch_stdout() { wget -qO- "$1"; }
else
	die "either curl or wget is required"
fi

if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	die "neither sha256sum nor shasum was found; cannot verify the download"
fi

if [ -z "${VERSION:-}" ]; then
	say "Resolving the newest release of $REPO…"
	VERSION=$(fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" \
		| tr ',' '\n' \
		| sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
		| head -n 1)
	[ -n "$VERSION" ] || die "could not resolve the latest release; set VERSION=vX.Y.Z to pin one"
fi

ARCHIVE="${BIN}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$VERSION"

say "Installing $BIN $VERSION ($OS/$ARCH) into $PREFIX"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

fetch "$BASE/$ARCHIVE"      "$tmp/$ARCHIVE"   || die "could not download $BASE/$ARCHIVE"
fetch "$BASE/checksums.txt" "$tmp/checksums"  || die "could not download $BASE/checksums.txt"

want=$(sed -n "s/^\([0-9a-f]\{64\}\)[[:space:]][[:space:]]*\**${ARCHIVE}$/\1/p" "$tmp/checksums" | head -n 1)
[ -n "$want" ] || die "checksums.txt does not mention $ARCHIVE — refusing to install an unverified binary"

got=$(sha256 "$tmp/$ARCHIVE")
if [ "$want" != "$got" ]; then
	warn ""
	warn "CHECKSUM MISMATCH — not installing."
	warn "  expected $want"
	warn "  got      $got"
	warn ""
	warn "The download was corrupted, truncated, or tampered with. Try again;"
	warn "if it happens twice, open an issue at https://github.com/$REPO/issues"
	exit 1
fi
say "Checksum OK ($got)"

tar -xzf "$tmp/$ARCHIVE" -C "$tmp"
[ -f "$tmp/$BIN" ] || die "the archive did not contain $BIN"

mkdir -p "$PREFIX"
chmod 0755 "$tmp/$BIN"
mv "$tmp/$BIN" "$PREFIX/$BIN.new"
mv "$PREFIX/$BIN.new" "$PREFIX/$BIN"

say ""
say "Installed $PREFIX/$BIN"
"$PREFIX/$BIN" version || true

case ":${PATH}:" in
	*":$PREFIX:"*) ;;
	*)
		say ""
		warn "$PREFIX is not on your PATH. Add this to your shell profile:"
		warn "    export PATH=\"$PREFIX:\$PATH\""
		warn "…or run it by full path: $PREFIX/$BIN init"
		;;
esac

say ""
say "Next:  $BIN init          # write the config and wire up Claude Code"
say "Then:  $BIN doctor        # if anything looks wrong"
