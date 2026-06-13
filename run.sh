#!/usr/bin/env bash
# terva-tasks launcher.
#
# terva installs an extension by copying/cloning the repo and running the `exec`
# from extension.json verbatim — it never compiles Go sources. This wrapper
# bridges that gap: on first launch (and after any source change) it builds the
# binary, then execs it so terva speaks the extension protocol to the real
# process. That lets `terva ext install <path|git-url>` work on any platform
# with a Go toolchain, without committing a platform-specific binary.
#
# IMPORTANT: stdout is the protocol wire. Every byte of build chatter must go to
# stderr (terva captures it to $TERVA_HOME/logs/ext-terva-tasks.log); a stray
# stdout write would corrupt the JSON stream.
set -euo pipefail
cd "$(dirname "$0")"

bin="./terva-tasks"

needs_build() {
	[ -x "$bin" ] || return 0
	# Rebuild if any Go source or the module files are newer than the binary,
	# so a pull that changes sources doesn't run a stale build.
	if [ -n "$(find . -name '*.go' -newer "$bin" -print -quit 2>/dev/null)" ]; then
		return 0
	fi
	if [ go.mod -nt "$bin" ]; then
		return 0
	fi
	if [ -f go.sum ] && [ go.sum -nt "$bin" ]; then
		return 0
	fi
	return 1
}

if needs_build; then
	if ! command -v go >/dev/null 2>&1; then
		echo "[terva-tasks] Go toolchain not found on PATH; cannot build the extension." >&2
		echo "[terva-tasks] Install Go 1.22+ (https://go.dev/dl/) and relaunch terva." >&2
		exit 1
	fi
	echo "[terva-tasks] building $bin (first launch or sources changed)…" >&2
	go build -o "$bin" . >&2
	echo "[terva-tasks] build complete." >&2
fi

exec "$bin" "$@"
