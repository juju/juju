// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"
	"github.com/juju/version"
)

// MinMajorUpgradeVersionValue defines the value to identify what the last
// version should be.
var MinMajorUpgradeVersionValue = map[int]string{
	// TODO(wallyworld) - change to 2.9.0 when 2.9.0 is released.
	3: "2.9-rc2",
}

// MinMajorUpgradeVersion defines the minimum version all models
// must be running before a major version upgrade.
var MinMajorUpgradeVersion = map[int]version.Number{
	3: version.MustParse(MinMajorUpgradeVersionValue[3]),
}

// UpgradeAllowed returns true if a major version upgrade is allowed
// for the specified from and to versions.
func UpgradeAllowed(from, to version.Number) (bool, version.Number, error) {
	if from.Major == to.Major {
		return true, version.Number{}, nil
	}
	// Downgrades not allowed.
	if from.Major > to.Major {
		return false, version.Number{}, nil
	}
	minVer, ok := MinMajorUpgradeVersion[to.Major]
	if !ok {
		return false, version.Number{}, errors.Errorf("unknown version %q", to)
	}
	// Allow upgrades from rc etc.
	from.Tag = ""
	return from.Compare(minVer) >= 0, minVer, nil
}
