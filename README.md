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

`Patch` returns an error wrapping `bindiff.ErrCorruptPatch` for any malformed patch.

The bsdiff format has no checksums, so with plain bsdiff a patch applied to the wrong file produces the wrong output without an error. bindiff appends the SHA-256 of the old and new data to every patch it makes. `Patch` refuses the wrong old data with `bindiff.ErrChecksumMismatch` before writing anything, and checks the result at the end. The reference `bspatch` ignores these 72 bytes, so the patches stay compatible. Patches without them, such as those from the reference `bsdiff`, apply without the checks.

Memory use:

- `Diff` holds both inputs in memory, plus a suffix array of the old data that needs about 16 bytes per input byte. Diffing a 43 MB binary takes about 9 seconds and under 1 GB.
- `Patch` holds the old data in memory and streams the new data to the writer. Applying that same 43 MB patch takes under 0.2 seconds and about 140 MB.

## Auto-updating your CLI

The `selfupdate` package lets a Go program update itself by applying a patch to its own executable. A user on the previous release downloads a small patch instead of the whole binary.

```go
import "go.ajitem.com/bindiff/selfupdate"

func main() {
	// Removes the old binary a Windows update leaves behind.
	_ = selfupdate.CleanupOld("")
	// ...
}

func update(ctx context.Context) error {
	u := &selfupdate.Updater{
		Source:    selfupdate.GitHub("owner", "repo"),
		PublicKey: publicKey, // from selfupdate.ParsePublicKey
	}

	up, err := u.Check(ctx)
	switch {
	case errors.Is(err, selfupdate.ErrUpToDate):
		return nil
	case errors.Is(err, selfupdate.ErrNoPatch):
		// No patch from this build, for example one several releases
		// old. Fall back to downloading the full release.
		return downloadFullRelease(ctx)
	case err != nil:
		return err
	}
	return u.Apply(ctx, up)
}
```

How it works:

- Each release publishes `bindiff-manifest.json`. It lists patches by binary, OS and architecture, with the SHA-256 of the binary each patch applies to, the binary it produces, and the patch itself.
- `Check` hashes the running executable and picks the entry made for exactly that binary. Version strings play no part in matching.
- `Apply` downloads the patch, checks every hash, writes the new binary next to the old one, and renames it into place. Windows does not allow replacing a running executable, so there the old binary is moved aside first and removed on the next start by `CleanupOld`.
- If any check fails, the executable is left untouched.

Sources:

- `selfupdate.GitHub` reads the latest GitHub release. `WithToken` covers private repositories and `WithBaseURL` covers GitHub Enterprise Server.
- `selfupdate.HTTP` reads from any static host, such as S3 or a CDN.
- Anything else can implement the two-method `selfupdate.Source` interface.

Signing:

- Run `bindiff keygen` to make an ed25519 key pair.
- Keep the private key as a CI secret and embed the public key in your program.
- When `Updater.PublicKey` is set, `Check` rejects any manifest without a valid signature.

If the executable sits in a directory the user cannot write to, such as one managed by a package manager, `Apply` returns an error wrapping `fs.ErrPermission`. Skip self-update for package-manager installs.

The bindiff command updates itself the same way: `bindiff self-update`.

## Publishing patches

The bindiff GitHub Action makes the patches and manifest for each release. With GoReleaser it is one extra step:

```yaml
- uses: asahasrabuddhe/bindiff@v0.3.0
  with:
    goreleaser-dist: dist
    signing-key: ${{ secrets.BINDIFF_SIGNING_KEY }}
```

See [docs/releasing-patches.md](docs/releasing-patches.md) for the full setup, other build tools, and other CI systems.

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
