// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/testing"
	"github.com/juju/utils/v4/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/provider/common"
)

var logger = internallogger.GetLogger("juju.environs.testing")

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
		logger.Infof(context.Background(), "provider/common.FinishBootstrap is disabled")
		return nil
	}
	return testing.PatchValue(&common.FinishBootstrap, f)
}

// BootstrapContext creates a simple bootstrap execution context.
func BootstrapContext(ctx context.Context, c *gc.C) environs.BootstrapContext {
	return environscmd.BootstrapContext(ctx, cmdtesting.Context(c))
}

func BootstrapTestContext(c *gc.C) environs.BootstrapContext {
	return BootstrapContext(context.Background(), c)
}
