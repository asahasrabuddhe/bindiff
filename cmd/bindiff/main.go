// Command bindiff creates binary patches and applies them.
package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"

	"github.com/urfave/cli/v3"

	"go.ajitem.com/bindiff"
	"go.ajitem.com/bindiff/internal/release"
	"go.ajitem.com/bindiff/selfupdate"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = ""

// updatePublicKey verifies release manifests for self-update. Release
// builds set it with -ldflags "-X main.updatePublicKey=...".
var updatePublicKey = ""

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)

	// Remove the binary a self-update on Windows left behind.
	_ = selfupdate.CleanupOld("")

	err := run(ctx, os.Args, os.Stdout, os.Stderr)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bindiff:", err)
		os.Exit(1)
	}
}

// buildVersion returns the version set by the linker, or the module version
// when built with go install.
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := func() []cli.Flag {
		return []cli.Flag{
			&cli.StringFlag{Name: "oldfile", Aliases: []string{"o"}, Usage: "path to the old `FILE`", Required: true},
			&cli.StringFlag{Name: "newfile", Aliases: []string{"n"}, Usage: "path to the new `FILE`", Required: true},
			&cli.StringFlag{Name: "patchfile", Aliases: []string{"p"}, Usage: "path to the patch `FILE`", Required: true},
		}
	}

	cmd := &cli.Command{
		Name:                  "bindiff",
		Usage:                 "create patches from or apply patches to binary files",
		Version:               buildVersion(),
		Authors:               []any{"Ajitem Sahasrabuddhe <ajitem.s@outlook.com>"},
		Copyright:             "© 2019 Ajitem Sahasrabuddhe",
		EnableShellCompletion: true,
		Writer:                stdout,
		ErrWriter:             stderr,
		Commands: []*cli.Command{
			{
				Name:    "diff",
				Aliases: []string{"d"},
				Usage:   "write a patch that turns OLDFILE into NEWFILE to PATCHFILE",
				Flags:   flags(),
				Action: func(_ context.Context, cmd *cli.Command) error {
					return diff(cmd.String("oldfile"), cmd.String("newfile"), cmd.String("patchfile"))
				},
			},
			{
				Name:    "patch",
				Aliases: []string{"p"},
				Usage:   "apply PATCHFILE to OLDFILE and write the result to NEWFILE",
				Flags:   flags(),
				Action: func(_ context.Context, cmd *cli.Command) error {
					return patch(cmd.String("oldfile"), cmd.String("newfile"), cmd.String("patchfile"))
				},
			},
			{
				Name:  "release",
				Usage: "make patches from earlier GitHub releases to this release's binaries",
				Description: "Reads the new binaries from GoReleaser's artifacts.json or from globs, downloads\n" +
					"the binaries of earlier releases, and writes patches and bindiff-manifest.json\n" +
					"for the selfupdate package. With --upload it attaches them to the release.",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "repo", Usage: "GitHub repository as `OWNER/NAME`", Sources: cli.EnvVars("GITHUB_REPOSITORY"), Required: true},
					&cli.StringFlag{Name: "tag", Usage: "release `TAG` the new binaries belong to", Sources: cli.EnvVars("GITHUB_REF_NAME"), Required: true},
					&cli.IntFlag{Name: "previous", Usage: "number of earlier releases to patch from", Value: 1},
					&cli.StringFlag{Name: "goreleaser", Usage: "GoReleaser `DIST` directory to read artifacts.json from"},
					&cli.StringSliceFlag{Name: "binaries", Usage: "`GLOB` matching new binaries, when not using GoReleaser"},
					&cli.StringFlag{Name: "out", Usage: "`DIR` to write patches and the manifest to", Value: "bindiff-patches"},
					&cli.StringFlag{Name: "signing-key-env", Usage: "environment `VARIABLE` holding the manifest signing key", Value: "BINDIFF_SIGNING_KEY"},
					&cli.StringFlag{Name: "github-api-url", Usage: "GitHub API `URL`", Sources: cli.EnvVars("GITHUB_API_URL"), Value: "https://api.github.com"},
					&cli.IntFlag{Name: "jobs", Usage: "number of diffs to run at once", Value: 2},
					&cli.BoolFlag{Name: "upload", Usage: "attach the patches and manifest to the release"},
				},
				Action: releaseCmd,
			},
			{
				Name:  "keygen",
				Usage: "print a new key pair for signing release manifests",
				Action: func(_ context.Context, cmd *cli.Command) error {
					return keygen(cmd.Root().Writer)
				},
			},
			{
				Name:  "self-update",
				Usage: "update bindiff to the latest release with a patch",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "check", Usage: "only report whether an update is available"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return selfUpdate(ctx, cmd.Root().Writer, cmd.Bool("check"))
				},
			},
		},
	}

	return cmd.Run(ctx, args)
}

func keygen(w io.Writer) error {
	pub, priv, err := selfupdate.GenerateKey()
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "Public key (embed in your program):\n%s\n\nPrivate key (store as a CI secret):\n%s\n", pub, priv)
	return err
}

func releaseCmd(ctx context.Context, cmd *cli.Command) error {
	var (
		bins []release.Binary
		err  error
	)
	switch dist, globs := cmd.String("goreleaser"), cmd.StringSlice("binaries"); {
	case dist != "" && len(globs) > 0:
		return errors.New("use either --goreleaser or --binaries, not both")
	case dist != "":
		bins, err = release.FromGoReleaser(dist)
	case len(globs) > 0:
		bins, err = release.FromGlobs(globs)
	default:
		return errors.New("one of --goreleaser or --binaries is required")
	}
	if err != nil {
		return err
	}

	var key ed25519.PrivateKey
	if v := os.Getenv(cmd.String("signing-key-env")); v != "" {
		if key, err = selfupdate.ParsePrivateKey(v); err != nil {
			return err
		}
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}

	res, err := release.Run(ctx, release.Config{
		GitHub:     &release.GitHub{BaseURL: cmd.String("github-api-url"), Token: token},
		Repo:       cmd.String("repo"),
		Tag:        cmd.String("tag"),
		Previous:   int(cmd.Int("previous")),
		Binaries:   bins,
		OutDir:     cmd.String("out"),
		SigningKey: key,
		Jobs:       int(cmd.Int("jobs")),
		Upload:     cmd.Bool("upload"),
		Log:        cmd.Root().Writer,
	})
	if err != nil {
		return err
	}

	// Report to a GitHub Actions step, if this runs in one.
	if out := os.Getenv("GITHUB_OUTPUT"); out != "" {
		count := 0
		if res.Manifest != nil {
			count = len(res.Manifest.Patches)
		}
		f, err := os.OpenFile(out, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		_, werr := fmt.Fprintf(f, "manifest=%s\npatch-count=%d\n", res.ManifestPath, count)
		if err := f.Close(); werr == nil {
			werr = err
		}
		return werr
	}
	return nil
}

const releasesURL = "https://github.com/asahasrabuddhe/bindiff/releases/latest"

func selfUpdate(ctx context.Context, w io.Writer, checkOnly bool) error {
	u := &selfupdate.Updater{
		Source: selfupdate.GitHub("asahasrabuddhe", "bindiff"),
		Binary: "bindiff",
	}
	if updatePublicKey != "" {
		key, err := selfupdate.ParsePublicKey(updatePublicKey)
		if err != nil {
			return err
		}
		u.PublicKey = key
	}

	up, err := u.Check(ctx)
	switch {
	case errors.Is(err, selfupdate.ErrUpToDate):
		_, err = fmt.Fprintf(w, "bindiff %s is up to date\n", buildVersion())
		return err
	case errors.Is(err, selfupdate.ErrNoPatch), errors.Is(err, selfupdate.ErrNotFound):
		_, err = fmt.Fprintf(w, "No patch from this build. Download the latest release from %s\n", releasesURL)
		return err
	case err != nil:
		return err
	}

	if checkOnly {
		_, err = fmt.Fprintf(w, "Update available: %s to %s (%d byte patch)\n", buildVersion(), up.Version, up.Entry.PatchSize)
		return err
	}

	if err := u.Apply(ctx, up); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Updated bindiff from %s to %s\n", buildVersion(), up.Version)
	return err
}

func diff(oldPath, newPath, patchPath string) error {
	oldFile, err := os.Open(oldPath)
	if err != nil {
		return err
	}
	defer oldFile.Close()

	newFile, err := os.Open(newPath)
	if err != nil {
		return err
	}
	defer newFile.Close()

	return writeAtomic(patchPath, 0o644, func(w io.Writer) error {
		return bindiff.Diff(oldFile, newFile, w)
	})
}

func patch(oldPath, newPath, patchPath string) error {
	oldFile, err := os.Open(oldPath)
	if err != nil {
		return err
	}
	defer oldFile.Close()

	patchFile, err := os.Open(patchPath)
	if err != nil {
		return err
	}
	defer patchFile.Close()

	// Keep the old file's mode, so patching an executable gives an
	// executable.
	mode := os.FileMode(0o644)
	if fi, err := oldFile.Stat(); err == nil {
		mode = fi.Mode().Perm()
	}

	return writeAtomic(newPath, mode, func(w io.Writer) error {
		return bindiff.Patch(oldFile, w, patchFile)
	})
}

// writeAtomic writes to a temporary file next to path and renames it into
// place only if write succeeds, so a failure never leaves a partial file.
func writeAtomic(path string, mode os.FileMode, write func(io.Writer) error) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	if err := write(tmp); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}
