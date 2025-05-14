// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/environs"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/testhelpers"
)

var logger = internallogger.GetLogger("juju.environs.testing")

// DisableFinishBootstrap disables common.FinishBootstrap so that tests
// do not attempt to SSH to non-existent machines. The result is a function
// that restores finishBootstrap.
func DisableFinishBootstrap(c *tc.C) func() {
	f := func(
		environs.BootstrapContext,
		ssh.Client,
		environs.Environ,
		instances.Instance,
		*instancecfg.InstanceConfig,
		environs.BootstrapDialOpts,
	) error {
		logger.Infof(c.Context(), "provider/common.FinishBootstrap is disabled")
		return nil
	}
	return testhelpers.PatchValue(&common.FinishBootstrap, f)
}

// BootstrapContext creates a simple bootstrap execution context.
func BootstrapContext(ctx context.Context, c *tc.C) environs.BootstrapContext {
	return environscmd.BootstrapContext(ctx, cmdtesting.Context(c))
}

func BootstrapTestContext(c *tc.C) environs.BootstrapContext {
	return BootstrapContext(c.Context(), c)
}
