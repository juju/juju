// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	"github.com/juju/utils/v3/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/environs"
	environscmd "github.com/juju/juju/environs/cmd"
	envcontext "github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
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
		environs.Environ,
		envcontext.ProviderCallContext,
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
func BootstrapContext(ctx context.Context, c *gc.C) environs.BootstrapContext {
	return environscmd.BootstrapContext(ctx, cmdtesting.Context(c))
}

func BootstrapTODOContext(c *gc.C) environs.BootstrapContext {
	return BootstrapContext(context.TODO(), c)
}
