// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import "github.com/juju/juju/version"

// Upgradeable represents a provider that supports upgrade steps
// if present, these steps will get called upon upgrading.
type Upgradeable interface {
	RunUpgradeStepsFor(version.Number) error
}
