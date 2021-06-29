// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/cmd/juju/commands"
	components "github.com/juju/juju/component/all"
	_ "github.com/juju/juju/provider/all" // Import the providers.
)

var log = loggo.GetLogger("juju.cmd.juju")

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func init() {
	if err := components.RegisterForClient(); err != nil {
		log.Criticalf("unable to register client components: %v", err)
		os.Exit(1)
	}
}

func main() {
	_, err := loggo.ReplaceDefaultWriter(cmd.NewWarningWriter(os.Stderr))
	if err != nil {
		panic(err)
	}
	os.Exit(commands.Main(os.Args))
}
