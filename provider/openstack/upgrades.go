// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/v3/environs"
)

// PreparePrechecker is part of the environs.JujuUpgradePrechecker
// interface. It is called to to give an Environ a chance to perform
// interactive operations that are required for prechecking
// an upgrade.
func (e *Environ) PreparePrechecker() error {
	return authenticateClient(e.client())
}

// PrecheckUpgradeOperations is part of the environs.JujuUpgradePrechecker
// interface.  It returns a slice of PrecheckJujuUpgradeOperation to be
// used to determine if a controller can be safely upgraded.
func (env *Environ) PrecheckUpgradeOperations() []environs.PrecheckJujuUpgradeOperation {
	return []environs.PrecheckJujuUpgradeOperation{{
		TargetVersion: version.MustParse("2.8.0"), // should be 2.8
		Steps: []environs.PrecheckJujuUpgradeStep{
			verifyNeutronEnabledStep{env},
		},
	}}
}

type verifyNeutronEnabledStep struct {
	env *Environ
}

func (verifyNeutronEnabledStep) Description() string {
	return "Verify Neutron OpenStack service enabled"
}

// Run is part of the environs.PrecheckJujuUpgradeStep interface.
func (step verifyNeutronEnabledStep) Run() error {
	if step.env.supportsNeutron() {
		return nil
	}
	return errors.NotFoundf("OpenStack Neutron service")
}
