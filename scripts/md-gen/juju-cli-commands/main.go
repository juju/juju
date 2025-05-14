// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"os"

	"github.com/juju/gnuflag"

	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/internal/cmd"
)

// This script generates Markdown documentation for the juju cli documentation.
// The first argument is the destination directory for the generated the
// documentation.
func main() {
	if len(os.Args) < 2 {
		panic("destination directory must be provided")
	}
	dest := os.Args[1]

	cmdCtx, err := cmd.DefaultContext()
	if err != nil {
		panic(err)
	}

	// Generate a new juju super command with all subcommands registered on it.
	// NewJujuCommandWithStore is initialised with a nil store as this is not
	// needed for generating the documentation.
	jujuCmd := commands.NewJujuCommandWithStore(cmdCtx, nil, nil, "", "", nil, false)

	jujuCmd.SetFlags(&gnuflag.FlagSet{})
	err = jujuCmd.Init([]string{"documentation", "--split", "--no-index", "--out", dest})
	if err != nil {
		panic(err)
	}

	err = jujuCmd.Run(&cmd.Context{
		Context: context.Background(),
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	if err != nil {
		panic(err)
	}
}
