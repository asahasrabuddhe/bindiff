// Command app is a minimal self-updating program for the end-to-end test.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.ajitem.com/bindiff/selfupdate"
)

var version = "dev"

func main() {
	if err := selfupdate.CleanupOld(""); err != nil {
		fmt.Fprintln(os.Stderr, "cleanup:", err)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(version)
	case "update":
		if err := update(); err != nil {
			fmt.Fprintln(os.Stderr, "update:", err)
			os.Exit(1)
		}
	}
}

func update() error {
	key, err := selfupdate.ParsePublicKey(os.Getenv("APP_UPDATE_KEY"))
	if err != nil {
		return err
	}

	ctx := context.Background()
	u := &selfupdate.Updater{
		Source:    selfupdate.HTTP(os.Getenv("APP_UPDATE_URL")),
		Binary:    "app",
		PublicKey: key,
	}

	up, err := u.Check(ctx)
	if errors.Is(err, selfupdate.ErrUpToDate) {
		fmt.Println("up to date")
		return nil
	}
	if err != nil {
		return err
	}

	if err := u.Apply(ctx, up); err != nil {
		return err
	}
	fmt.Println("updated to", up.Version)
	return nil
}
