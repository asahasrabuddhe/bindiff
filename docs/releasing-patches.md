# Publishing patches with each release

Programs that use the `selfupdate` package need each release to publish patches from earlier releases and a `bindiff-manifest.json`. The bindiff GitHub Action does this after your release is built.

## With GoReleaser

Add one step after GoReleaser in the same job. The action reads `dist/artifacts.json`, so GoReleaser needs no extra configuration.

```yaml
name: Release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v7
        with:
          go-version: stable
      - uses: goreleaser/goreleaser-action@v7
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - uses: asahasrabuddhe/bindiff@v0.3.0
        with:
          goreleaser-dist: dist
          previous: 2
          signing-key: ${{ secrets.BINDIFF_SIGNING_KEY }}
```

The action downloads the archives of the previous two releases, finds the binaries inside them, and makes a patch from each to the matching new binary. It detects each binary's platform from its executable header, so any archive naming works. It then uploads the patches, the manifest and its signature to the new release.

Notes:

- **Draft releases.** The action can upload to a draft. Publish the release after the action finishes.
- **macOS signing and notarization.** GoReleaser signs binaries in place, and the action reads them from `dist` after GoReleaser finishes, so the patches produce the signed binaries users download.
- **amd64 microarchitecture levels.** Only the `v1` build gets patches. Users of other levels get `ErrNoPatch` and fall back to a full download.
- **First release.** With no earlier release there is nothing to patch from. The action logs that and succeeds.
- **Memory.** Diffing needs about 16 bytes per byte of the old binary. A 50 MB binary needs about 1 GB per diff. `jobs` sets how many run at once.

## Without GoReleaser

Point `binaries` at the files you built, one glob per line:

```yaml
- uses: asahasrabuddhe/bindiff@v0.3.0
  with:
    binaries: |
      build/*/myapp
      build/*/myapp.exe
```

## Without GitHub Actions

Run the command directly in any CI:

```sh
export GITHUB_TOKEN=...
export BINDIFF_SIGNING_KEY=...
bindiff release --repo owner/name --tag v1.2.0 --goreleaser dist --previous 2 --upload
```

## Signing

Make a key pair once:

```sh
bindiff keygen
```

Store the private key as the `BINDIFF_SIGNING_KEY` repository secret. Embed the public key in your program and set `Updater.PublicKey`. Every manifest then needs a valid signature, so a compromised release asset cannot push an update.

## Action inputs

| Input | Default | Description |
| --- | --- | --- |
| `goreleaser-dist` | | GoReleaser dist directory. Set this or `binaries`. |
| `binaries` | | Globs matching the new binaries, one per line. |
| `previous` | `1` | How many earlier releases to patch from. |
| `tag` | `github.ref_name` | Release the new binaries belong to. |
| `repo` | `github.repository` | Repository as owner/name. |
| `signing-key` | | Private key from `bindiff keygen`. |
| `github-token` | `github.token` | Needs `contents: write` to upload. |
| `upload` | `true` | Attach the files to the release. |
| `out` | `bindiff-patches` | Directory to write the files to. |
| `jobs` | `2` | Diffs to run at once. |
| `version` | the action's tag | bindiff release to download when Go is not installed. |

Outputs are `manifest`, the path of the manifest, and `patch-count`.
