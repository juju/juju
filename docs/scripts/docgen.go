// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	"github.com/juju/juju/cmd/juju/commands"
)

func main() {
	args := append([]string{os.Args[0], "documentation"}, os.Args[1:]...)
	os.Exit(commands.Main(args))
}
