// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"flag"
	"testing"

	gc "launchpad.net/gocheck"

	jujucmd "github.com/juju/juju/cmd/juju"
	cmdtesting "github.com/juju/juju/cmd/testing"
	_ "github.com/juju/juju/provider/dummy" // XXX Why?
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *testing.T) {
	if *cmdtesting.FlagRunMain {
		jujucmd.Main(flag.Args())
	}
}
