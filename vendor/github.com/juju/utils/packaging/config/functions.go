// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/utils/packaging"
)

// SeriesRequiresCloudArchiveTools signals whether the given series
// requires the configuration of cloud archive cloud tools.
func SeriesRequiresCloudArchiveTools(series string) bool {
	return seriesRequiringCloudTools[series]
}

// GetCloudArchiveSource returns the PackageSource and associated
// PackagePreferences for the cloud archive for the given series.
func GetCloudArchiveSource(series string) (packaging.PackageSource, packaging.PackagePreferences) {
	// TODO (aznashwan): find a more deterministic way of filtering series that
	// does not imply importing version from core.
	switch series {
	case "centos7":
		// NOTE: as of yet, the below function does nothing for CentOS.
		return configureCloudArchiveSourceCentOS(series)
	default:
		return configureCloudArchiveSourceUbuntu(series)
	}
}

func RequiresBackports(series string, pkg string) bool {
	backportPkgs := backportsBySeries[series]

	for _, backportPkg := range backportPkgs {
		if pkg == backportPkg {
			return true
		}
	}

	return false
}
