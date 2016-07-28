// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import "github.com/juju/juju/cloud"

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

	// StorageEndpoint is the storage endpoint for the cloud (region).
	StorageEndpoint string

	// Credential is the cloud credential to use to authenticate
	// with the cloud, or nil if the cloud does not require any
	// credentials.
	Credential *cloud.Credential
}
