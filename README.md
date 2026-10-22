# bindiff

[![CI](https://github.com/asahasrabuddhe/bindiff/actions/workflows/ci.yml/badge.svg)](https://github.com/asahasrabuddhe/bindiff/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/go.ajitem.com/bindiff.svg)](https://pkg.go.dev/go.ajitem.com/bindiff)

bindiff creates and applies binary patches in the [bsdiff](https://www.daemonology.net/bsdiff/) 4.x format, in pure Go.

Patches are compatible with the reference tools in both directions. The reference `bspatch` applies patches made by bindiff, and bindiff applies patches made by the reference `bsdiff`. The test suite checks both.

A patch between two builds of the same program is usually a small fraction of the full binary. For example, the patch from `gh` v2.62.0 to v2.63.0 is 1.4 MB, while the new binary is 43 MB.

## Install

Download a binary for your platform from the [releases page](https://github.com/asahasrabuddhe/bindiff/releases), or build from source:

```sh
go install go.ajitem.com/bindiff/cmd/bindiff@latest
```

## Command line

Create a patch:

```sh
bindiff diff --oldfile app-v1 --newfile app-v2 --patchfile app-v1-v2.patch
```

Apply it:

```sh
bindiff patch --oldfile app-v1 --newfile app-v2 --patchfile app-v1-v2.patch
```

The short flags are `-o`, `-n` and `-p`. Output files are written to a temporary file and renamed into place, so a failed run never leaves a partial file. `bindiff patch` gives the new file the same permissions as the old one.

## Library

```go
import "go.ajitem.com/bindiff"

// Create a patch.
err := bindiff.Diff(oldReader, newReader, patchWriter)

// Apply it.
err = bindiff.Patch(oldReader, newWriter, patchReader)
```

`Patch` returns an error wrapping `bindiff.ErrCorruptPatch` for any malformed patch. The bsdiff format has no checksum of the old data, so applying a patch to the wrong file produces the wrong output without an error. Check a hash of the input or the output when that matters.

Memory use:

- `Diff` holds both inputs in memory, plus a suffix array of the old data that needs about 16 bytes per input byte. Diffing a 43 MB binary takes about 9 seconds and under 1 GB.
- `Patch` holds the old data in memory and streams the new data to the writer. Applying that same 43 MB patch takes under 0.2 seconds and about 140 MB.

## Development

```sh
go test ./...
go test -run '^$' -bench . -benchmem .
go test -run '^$' -fuzz FuzzPatch .
golangci-lint run
```

`testdata/golden` holds a corpus of inputs with the patches bindiff and the reference bsdiff 4.3 produce for them. `testdata/golden/gen.sh` regenerates it, and `go test -run TestGoldenDiff -update` refreshes only bindiff's patches.

## License

MIT
