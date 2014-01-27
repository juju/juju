// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/ssh"
)

var logger = loggo.GetLogger("juju.environs.testing")

// DisableFinishBootstrap disables common.FinishBootstrap so that tests
// do not attempt to SSH to non-existent machines. The result is a function
// that restores finishBootstrap.
func DisableFinishBootstrap() func() {
	f := func(environs.BootstrapContext, ssh.Client, instance.Instance, *cloudinit.MachineConfig) error {
		logger.Warningf("provider/common.FinishBootstrap is disabled")
		return nil
	}
	return testbase.PatchValue(&common.FinishBootstrap, f)
}

type bootstrapContext struct {
	*cmd.Context
}

func (c bootstrapContext) Stdin() io.Reader {
	return c.Context.Stdin
}

func (c bootstrapContext) Stdout() io.Writer {
	return c.Context.Stdout
}

func (c bootstrapContext) Stderr() io.Writer {
	return c.Context.Stderr
}

func NewBootstrapContext(ctx *cmd.Context) environs.BootstrapContext {
	return bootstrapContext{ctx}
}
