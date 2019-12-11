package main

import (
	"github.com/urfave/cli/v2"
	"go.ajitem.com/bindiff"
	"log"
	"os"
)

var Version string

func main() {
	app := cli.NewApp()

	app.Name = "bindiff"
	app.Usage = "Simple tool to create or apply patch of difference in two binary files"

	app.Authors = []*cli.Author{
		{
			Name:  "Ajitem Sahasrabuddhe",
			Email: "ajitem.s@outlook.com",
		},
	}

	app.Version = Version

	app.Flags = []cli.Flag{
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
			Action: func(context *cli.Context) error {
				oldFile, newFile, patchFile, err := getFilesFromContext(context, true)
				if err != nil {
					return err
				}

				return bindiff.Diff(oldFile, newFile, patchFile)
			},
		},
		{
			Name:    "patch",
			Aliases: []string{"p"},
			Action: func(context *cli.Context) error {
				oldFile, newFile, patchFile, err := getFilesFromContext(context, false)
				if err != nil {
					return err
				}

				return bindiff.Patch(oldFile, newFile, patchFile)
			},
		},
	}

	log.Fatal(app.Run(os.Args))
}

func getFilesFromContext(context *cli.Context, diff bool) (oldFile *os.File, newFile *os.File, patchFile *os.File, err error) {
	oldFile, err = os.Open(context.Path("oldfile"))
	if err != nil {
		return nil, nil, nil, err
	}

	if diff {
		newFile, err = os.Open(context.Path("newfile"))
		if err != nil {
			return nil, nil, nil, err
		}

		patchFile, err = os.Create(context.Path("patchfile"))
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		newFile, err = os.Create(context.Path("newfile"))
		if err != nil {
			return nil, nil, nil, err
		}

		patchFile, err = os.Open(context.Path("patchfile"))
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return oldFile, newFile, patchFile, nil
}
