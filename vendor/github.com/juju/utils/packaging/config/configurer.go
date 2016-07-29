// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/errors"
)

// baseConfigurer is the base type of a Configurer object.
type baseConfigurer struct {
	series               string
	defaultPackages      []string
	cloudArchivePackages map[string]struct{}
}

// DefaultPackages is defined on the PackagingConfigurer interface.
func (c *baseConfigurer) DefaultPackages() []string {
	return c.defaultPackages
}

// GetPackageNameForSeries is defined on the PackagingConfigurer interface.
func (c *baseConfigurer) GetPackageNameForSeries(pack, series string) (string, error) {
	if c.series == series {
		return pack, nil
	}

	// TODO(aznashwan): find a more deterministic way of filtering series that
	// does not imply importing version from core.
	switch series {
	case "centos7":
		res, ok := centOSToUbuntuPackageNameMap[pack]
		if !ok {
			return "", errors.Errorf("no equivalent package found for series %s: %s", series, pack)
		}
		return res, nil
	default:
		res, ok := ubuntuToCentOSPackageNameMap[pack]
		if !ok {
			return "", errors.Errorf("no equivalent package found for series %s: %s", series, pack)
		}
		return res, nil
	}

	return pack, nil
}

// IsCloudArchivePackage is defined on the PackagingConfigurer interface.
func (c *baseConfigurer) IsCloudArchivePackage(pack string) bool {
	_, ok := c.cloudArchivePackages[pack]
	return ok
}
