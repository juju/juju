// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/internal/debug/coveruploader"
	_ "github.com/juju/juju/provider/all" // Import the providers.
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func main() {
	coveruploader.Enable()
	_, err := loggo.ReplaceDefaultWriter(cmd.NewWarningWriter(os.Stderr))
	if err != nil {
		panic(err)
	}
	os.Exit(commands.Main(os.Args))
}
