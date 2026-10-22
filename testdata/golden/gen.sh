#!/bin/sh
# Regenerates the golden corpus. Run from the repository root:
#
#   testdata/golden/gen.sh
#
# Inputs come from gen/main.go. patch.bsdiff files come from the reference
# bsdiff 4.3: a local bsdiff if one is on PATH, otherwise Ubuntu's package in
# Docker. bsdiff 4.3 fails on empty files, so those cases have no
# patch.bsdiff. patch.bindiff files come from this package's Diff.
set -eu

root=testdata/golden

go run ./$root/gen -root $root

if command -v bsdiff >/dev/null 2>&1; then
	for dir in $root/*/; do
		[ -f "$dir/old" ] || continue
		bsdiff "$dir/old" "$dir/new" "$dir/patch.bsdiff" || echo "skipped $dir"
	done
else
	docker run --rm -v "$PWD/$root:/golden" ubuntu:24.04 sh -c '
		apt-get update -qq >/dev/null &&
		apt-get install -y -qq bsdiff >/dev/null &&
		for dir in /golden/*/; do
			[ -f "$dir/old" ] || continue
			bsdiff "$dir/old" "$dir/new" "$dir/patch.bsdiff" || echo "skipped $dir"
		done'
fi

go test -run TestGoldenDiff -update .
