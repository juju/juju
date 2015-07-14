// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	"github.com/juju/juju/cmd/juju/commands"
	// Import the providers.
	_ "github.com/juju/juju/provider/all"
)

func main() {
	commands.Main(os.Args)
}
