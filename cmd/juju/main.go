// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	"github.com/juju/juju/cmd/juju/commands"
	components "github.com/juju/juju/component/all"
	// Import the providers.
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/utils"
)

func init() {
	utils.Must(components.RegisterForClient())
}

func main() {
	commands.Main(os.Args)
}
