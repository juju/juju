// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
	names "gopkg.in/juju/names.v2"
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

	// StorageEndpoint is the storage endpoint for the cloud (region).
	StorageEndpoint string

	// Credential is the cloud credential to use to authenticate
	// with the cloud, or nil if the cloud does not require any
	// credentials.
	Credential *cloud.Credential
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
