// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"
	names "gopkg.in/juju/names.v2"

	jujucloud "github.com/juju/juju/cloud"
)

// CloudSpec describes a specific cloud configuration, for the purpose
// of opening an Environ to manage the cloud resources.
type CloudSpec struct {
	// Type is the type of cloud, eg aws, openstack etc.
	Type string

	// Name is the name of the cloud.
	Name string

	// Region is the name of the cloud region, if the cloud supports
	// regions.
	Region string

	// Endpoint is the endpoint for the cloud (region).
	Endpoint string

	// IdentityEndpoint is the identity endpoint for the cloud (region).
	IdentityEndpoint string

	// StorageEndpoint is the storage endpoint for the cloud (region).
	StorageEndpoint string

	// Credential is the cloud credential to use to authenticate
	// with the cloud, or nil if the cloud does not require any
	// credentials.
	Credential *jujucloud.Credential
}

// Validate validates that the CloudSpec is well-formed. It does
// not ensure that the cloud type and credentials are valid.
func (cs CloudSpec) Validate() error {
	if cs.Type == "" {
		return errors.NotValidf("empty Type")
	}
	if !names.IsValidCloud(cs.Name) {
		return errors.NotValidf("cloud name %q", cs.Name)
	}
	return nil
}

// MakeCloudSpec returns a CloudSpec from the given
// Cloud, cloud and region names, and credential.
func MakeCloudSpec(cloud jujucloud.Cloud, cloudName, cloudRegionName string, credential *jujucloud.Credential) (CloudSpec, error) {
	cloudSpec := CloudSpec{
		Type:             cloud.Type,
		Name:             cloudName,
		Region:           cloudRegionName,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		Credential:       credential,
	}
	if cloudRegionName != "" {
		cloudRegion, err := jujucloud.RegionByName(cloud.Regions, cloudRegionName)
		if err != nil {
			return CloudSpec{}, errors.Annotate(err, "getting cloud region definition")
		}
		cloudSpec.Endpoint = cloudRegion.Endpoint
		cloudSpec.IdentityEndpoint = cloudRegion.IdentityEndpoint
		cloudSpec.StorageEndpoint = cloudRegion.StorageEndpoint
	}
	return cloudSpec, nil
}

// RegionSpec contains the information needed to lookup specific region
// configuration. This is for use in calling
// state/modelconfig.(ComposeNewModelConfig) so there is no need to serialize
// it.
type RegionSpec struct {
	// Cloud is the name of the cloud.
	Cloud string

	// Region is the name of the cloud region.
	Region string
}
