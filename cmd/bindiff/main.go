package main

import (
	"log"
	"os"
	"runtime"
	"time"

	"github.com/urfave/cli/v2"
	"go.ajitem.com/bindiff"
)

var Version string

func main() {
	app := cli.NewApp()

	app.Name = "bindiff"
	app.Usage = "Simple tool to create patches from or apply patches to binary files"

	app.Authors = []*cli.Author{
		{
			Name:  "Ajitem Sahasrabuddhe",
			Email: "ajitem.s@outlook.com",
		},
	}

	app.EnableBashCompletion = true
	app.Version = Version
	app.Compiled = time.Now()
	app.Copyright = "© 2019 Ajitem Sahasrabuddhe"
	app.Metadata = map[string]interface{}{
		"name":    app.Name,
		"version": app.Version,
		"arch":    runtime.GOARCH,
		"os":      runtime.GOOS,
	}

	flags := []cli.Flag{
		&cli.PathFlag{
			Name:     "oldfile",
			Aliases:  []string{"o"},
			Required: true,
		},
		&cli.PathFlag{
			Name:     "newfile",
			Aliases:  []string{"n"},
			Required: true,
		},
		&cli.PathFlag{
			Name:     "patchfile",
			Aliases:  []string{"p"},
			Required: true,
		},
	}

	app.Commands = []*cli.Command{
		{
			Name:    "diff",
			Aliases: []string{"d"},
			Usage:   "Calculates the diff in between the `OLDFILE` and the `NEWFILE` and writes it into the `PATCHFILE`",
			Flags:   flags,
			Action: func(context *cli.Context) error {
				oldFile, newFile, patchFile, err := getFilesFromContext(context)
				if err != nil {
					return err
				}

				return bindiff.Diff(oldFile, newFile, patchFile)
			},
		},
		{
			Name:    "patch",
			Aliases: []string{"p"},
			Usage:   "Applies the `PATCHFILE` to the `OLDFILE` to create a `NEWFILE`",
			Flags:   flags,
			Action: func(context *cli.Context) error {
				oldFile, newFile, patchFile, err := getFilesFromContext(context)
				if err != nil {
					return err
				}

				return bindiff.Patch(oldFile, newFile, patchFile)
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func getFilesFromContext(context *cli.Context) (oldFile *os.File, newFile *os.File, patchFile *os.File, err error) {
	oldFile, err = os.Open(context.Path("oldfile"))
	if err != nil {
		return
	}

	if context.Command.Name == "diff" || context.Command.Name == "d" {
		newFile, err = os.Open(context.Path("newfile"))
		if err != nil {
			return
		}

		patchFile, err = os.Create(context.Path("patchfile"))
		if err != nil {
			return
		}
	} else {
		newFile, err = os.Create(context.Path("newfile"))
		if err != nil {
			return
		}

		patchFile, err = os.Open(context.Path("patchfile"))
		if err != nil {
			return
		}
	}

	return
}
