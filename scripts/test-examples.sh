#!/usr/bin/env bash
# Run the root suite and every nested example/debugger-* module, then the
# real-browser E2E. Requires Go 1.24+ (nested modules may request a newer
# toolchain), Node >=22 and Chrome/Chromium for the browser test.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)

echo "== root module"
(cd "$root" && go test ./... && go vet ./...)

for mod in "$root"/example/debugger-*; do
	[ -f "$mod/go.mod" ] || continue
	echo "== $(basename "$mod")"
	(cd "$mod" && go test ./... && go vet ./...)
done

echo "== e2e fixture build"
(cd "$root" && go -C e2e test ./...)

echo "== browser e2e"
(cd "$root" && node e2e/run.mjs)
