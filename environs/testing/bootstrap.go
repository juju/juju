// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	"github.com/juju/utils/v2/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	instances "github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/common"
)

var logger = loggo.GetLogger("juju.environs.testing")

// DisableFinishBootstrap disables common.FinishBootstrap so that tests
// do not attempt to SSH to non-existent machines. The result is a function
// that restores finishBootstrap.
func DisableFinishBootstrap() func() {
	f := func(
		environs.BootstrapContext,
		ssh.Client,
		common.HostSSHOptionsFunc,
		environs.Environ,
		context.ProviderCallContext,
		instances.Instance,
		*instancecfg.InstanceConfig,
		environs.BootstrapDialOpts,
	) error {
		logger.Infof("provider/common.FinishBootstrap is disabled")
		return nil
	}
	return testing.PatchValue(&common.FinishBootstrap, f)
}

// BootstrapContext creates a simple bootstrap execution context.
func BootstrapContext(c *gc.C) environs.BootstrapContext {
	return modelcmd.BootstrapContext(cmdtesting.Context(c))
}
