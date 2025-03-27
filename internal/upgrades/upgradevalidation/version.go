// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/semversion"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.upgrades.validations")

// MinAgentVersions defines the minimum agent version
// allowed to make a call to a controller with the major version.
var MinAgentVersions = map[int]semversion.Number{
	3: semversion.MustParse("2.9.43"),
}

// UpgradeControllerAllowed returns true if a controller upgrade is allowed
// when it hosts a model with the specified version.
func UpgradeControllerAllowed(modelVersion, targetControllerVersion semversion.Number) (bool, semversion.Number, error) {
	return versionCheck(modelVersion, targetControllerVersion, MinAgentVersions)
}

func versionCheck(
	from, to semversion.Number, versionMap map[int]semversion.Number,
) (bool, semversion.Number, error) {
	// If the major version is the same then we will allow the upgrade.
	if from.Major == to.Major {
		return true, semversion.Number{}, nil
	}
	// Downgrades not allowed.
	if from.Major > to.Major {
		logger.Debugf(context.TODO(), "downgrade from %q to %q is not allowed", from, to)
		return false, semversion.Number{}, errors.Errorf("downgrade is not allowed")
	}

	minVer, ok := versionMap[to.Major]
	logger.Debugf(context.TODO(), "from %q, to %q, versionMap %#v", from, to, versionMap)
	if !ok {
		return false, semversion.Number{}, errors.Errorf("%s to %q is not supported from %q", "upgrading controller", to, from)
	}
	// Allow upgrades from rc etc.
	from.Tag = ""
	return from.Compare(minVer) >= 0, minVer, nil
}
