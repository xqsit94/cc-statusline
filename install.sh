#!/bin/sh
# cc-statusline installer — PRD §10.1.
#
#   curl -fsSL https://raw.githubusercontent.com/xqsit94/cc-statusline/main/install.sh | sh
#
# What it does, and what it deliberately does not:
#
#   * Resolves the newest release tag ONCE, then uses that exact tag for both
#     the archive and the checksums. GitHub's /releases/latest/download/ URL is
#     a redirect that is re-resolved per request, so a release published
#     between the two downloads would have you verifying one release's binary
#     against another release's checksums and seeing a mismatch you cannot
#     explain. §10.1 calls this "the release tag is pinned rather than resolved
#     as latest at install time".
#
#   * Verifies sha256 before unpacking, and aborts loudly on a mismatch.
#     THIS IS INTEGRITY, NOT AUTHENTICITY. checksums.txt ships from the same
#     release as the binary, so it defends against a truncated or corrupted
#     download and against nothing else. Signing is deferred — PRD §13.
#
#   * Does NOT touch ~/.claude/settings.json. That is `cc-statusline init`,
#     which backs the file up first and declines to edit one it cannot edit
#     safely. An installer that silently rewired your status line would be a
#     poor introduction to a tool whose whole argument is not surprising you.
#
# Environment:
#   PREFIX    where to install       (default: $XDG_BIN_HOME, else ~/.local/bin)
#   VERSION   which tag to install   (default: the newest release)
#   REPO      which repository       (default: xqsit94/cc-statusline)

set -eu

REPO="${REPO:-xqsit94/cc-statusline}"
BIN="cc-statusline"

say()  { printf '%s\n' "$*"; }
warn() { printf '%s\n' "$*" >&2; }
die()  { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is required but was not found"
}

# ── Where ────────────────────────────────────────────────────────────────────
if [ -z "${PREFIX:-}" ]; then
	if [ -n "${XDG_BIN_HOME:-}" ]; then
		PREFIX="$XDG_BIN_HOME"
	else
		PREFIX="$HOME/.local/bin"
	fi
fi

# ── What ─────────────────────────────────────────────────────────────────────
os=$(uname -s)
arch=$(uname -m)

case "$os" in
	Linux)  OS=Linux ;;
	Darwin) OS=Darwin ;;
	# §13 defers Windows: git discovery, XDG paths, and the install prefix each
	# need an answer nobody has written. Failing here is honest; installing
	# something untested would not be.
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

# sha256sum on GNU systems, shasum on macOS. Both are checked for up front
# rather than at the point of use, so a missing one fails before anything has
# been downloaded or written.
if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	die "neither sha256sum nor shasum was found; cannot verify the download"
fi

# ── Which version ────────────────────────────────────────────────────────────
if [ -z "${VERSION:-}" ]; then
	say "Resolving the newest release of $REPO…"
	# Read the tag out of the API response without needing jq. The field is
	# emitted once per response, so the first match is the release's own tag.
	VERSION=$(fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" \
		| tr ',' '\n' \
		| sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
		| head -n 1)
	[ -n "$VERSION" ] || die "could not resolve the latest release; set VERSION=vX.Y.Z to pin one"
fi

ARCHIVE="${BIN}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$VERSION"

say "Installing $BIN $VERSION ($OS/$ARCH) into $PREFIX"

# ── Download and verify ──────────────────────────────────────────────────────
tmp=$(mktemp -d)
# The trap covers every exit path including the failures below, so a bad
# checksum leaves nothing behind to be picked up and run later by accident.
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

# ── Install ──────────────────────────────────────────────────────────────────
tar -xzf "$tmp/$ARCHIVE" -C "$tmp"
[ -f "$tmp/$BIN" ] || die "the archive did not contain $BIN"

mkdir -p "$PREFIX"
# Installed via a temporary name and renamed, so that upgrading while a status
# line is mid-render replaces the file rather than truncating the one being
# executed.
chmod 0755 "$tmp/$BIN"
mv "$tmp/$BIN" "$PREFIX/$BIN.new"
mv "$PREFIX/$BIN.new" "$PREFIX/$BIN"

say ""
say "Installed $PREFIX/$BIN"
"$PREFIX/$BIN" version || true

# ~/.local/bin is not on the default PATH on macOS or most Linux distributions,
# and this is worth saying out loud: Claude Code runs the status line through a
# non-interactive shell that never sources a profile. `init` writes the absolute
# path for exactly that reason, so the status line works either way — but the
# user typing `cc-statusline init` next needs it on PATH.
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
