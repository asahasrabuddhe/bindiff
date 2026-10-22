// Command bindiff creates binary patches and applies them.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"

	"github.com/urfave/cli/v3"

	"go.ajitem.com/bindiff"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = ""

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)

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
		},
	}

	return cmd.Run(ctx, args)
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
