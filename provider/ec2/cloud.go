// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

type environProviderCloud struct{}

// FinalizeCloud implements environs.CloudFinalizer.FinalizeCloud
func (environProviderCloud) FinalizeCloud(
	ctx environs.FinalizeCloudContext,
	cld cloud.Cloud,
) (cloud.Cloud, error) {
	// We want to make sure that the cloud at least has the instance role
	// auth type in it's supported list as there may be alterations to a
	// credential that forces this new type. Specifically this could happen
	// during bootstrap. By adding it to the always supported list we are
	// avoiding things blowing up. Added by tlm on 07/10/2021
	if !cld.AuthTypes.Contains(cloud.InstanceRoleAuthType) {
		cld.AuthTypes = append(cld.AuthTypes, cloud.InstanceRoleAuthType)
	}

	return cld, nil
}
