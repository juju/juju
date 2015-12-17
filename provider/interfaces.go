// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import "github.com/juju/juju/version"

// Upgradable represents a provider that supports upgrade steps
// if present, these steps will get called upon upgrading.
type Upgradable interface {
	RunUpgradeStepsFor(version.Number) error
}
