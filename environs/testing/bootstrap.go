// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

var logger = loggo.GetLogger("juju.environs.testing")

// DisableFinishBootstrap disables common.FinishBootstrap so that tests
// do not attempt to SSH to non-existent machines. The result is a function
// that restores finishBootstrap.
func DisableFinishBootstrap() func() {
	f := func(environs.BootstrapContext, instance.Instance, *cloudinit.MachineConfig) error {
		logger.Warningf("provider/common.FinishBootstrap is disabled")
		return nil
	}
	return testbase.PatchValue(&common.FinishBootstrap, f)
}

type BootstrapContext struct {
	*cmd.Context
}

func (c BootstrapContext) Stdin() io.Reader {
	return c.Context.Stdin
}

func (c BootstrapContext) Stdout() io.Writer {
	return c.Context.Stdout
}

func (c BootstrapContext) Stderr() io.Writer {
	return c.Context.Stderr
}

func NewBootstrapContext(c *gc.C) BootstrapContext {
	return BootstrapContext{coretesting.Context(c)}
}
