// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	cmdtesting "github.com/juju/juju/cmd/testing"
	_ "github.com/juju/juju/provider/dummy" // XXX Why?
	"github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func badrun(c *gc.C, exit int, args ...string) string {
	args = append([]string{"juju"}, args...)
	return cmdtesting.BadRun(c, exit, args...)
}

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *cmdtesting.FlagRunMain {
		Main(flag.Args())
	}
}
