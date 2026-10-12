// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/semversion"
	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
)

// CheckAgentVersion enforces the exact-version recovery gate: an archive
// is only ever recovered onto the same agent version it was taken from,
// build number included.
func CheckAgentVersion(i *domainrecovery.ArchiveInfo, current semversion.Number) error {
	if i.AgentVersion.Compare(current) != 0 {
		return errors.Errorf(
			"archive was created by agent version %s but this binary is %s; "+
				"recovery requires the exact same version", i.AgentVersion, current)
	}
	return nil
}

// CheckProviderFamily enforces the same-provider-family rule: the
// replacement controller must be bootstrapped on a cloud of the same
// provider type as the source controller model's cloud.
func CheckProviderFamily(i *domainrecovery.ArchiveInfo, targetCloudType string) error {
	if i.CloudType != targetCloudType {
		return errors.Errorf(
			"archive comes from a %q controller but the bootstrap cloud is %q; "+
				"recovery requires the same provider family", i.CloudType, targetCloudType)
	}
	return nil
}
