package main

import (
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/asimonigh/mockroute-cli/internal/adb"
	"github.com/asimonigh/mockroute-cli/internal/cli"
)

var version = "dev"

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	env := cli.Env{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Locator: adb.Locator{
			Getenv: os.Getenv,
			GOOS:   runtime.GOOS,
			Home:   home,
			Exists: func(p string) bool {
				info, err := os.Stat(p)
				return err == nil && !info.IsDir()
			},
			LookPath: exec.LookPath,
		},
		NewRunner: func(path string) adb.Runner { return adb.ExecRunner{Path: path} },
		Sleep:     time.Sleep,
		Now:       time.Now,
		Version:   version,
	}
	os.Exit(cli.Run(os.Args[1:], env))
}
