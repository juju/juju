// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/testing/testbase"
)

// DisableFinishBootstrap disables common.FinishBootstrap so that tests
// do not attempt to SSH to non-existent machines. The result is a function
// that restores finishBootstrap.
func DisableFinishBootstrap() func() {
	f := func(*common.BootstrapContext, instance.Instance, *cloudinit.MachineConfig) error {
		return nil
	}
	return testbase.PatchValue(&common.FinishBootstrap, f)
}
