// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/provider/openstack"
)

// Credentials represents openstack credentials specifically tailored
// to rackspace.  Mostly this means that they're appropriate for the v2 API, and
// thus there's no domain name.
type Credentials struct {
	openstack.OpenstackCredentials
}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (c Credentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	m := c.OpenstackCredentials.CredentialSchemas()
	schema := m[cloud.UserPassAuthType]
	// remove domain name from attributes.
	for i, attr := range schema {
		if attr.Name == openstack.CredAttrDomainName {
			m[cloud.UserPassAuthType] = append(schema[:i], schema[i+1:]...)
			break
		}
	}
	return m
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (c Credentials) DetectCredentials() (*cloud.CloudCredential, error) {
	result, err := c.OpenstackCredentials.DetectCredentials()
	if err != nil {
		return nil, err
	}
	// delete domain name from creds, since rackspace doesn't use it, and it
	// confuses our code.
	for k, v := range result.AuthCredentials {
		attr := v.Attributes()
		if _, ok := attr[openstack.CredAttrDomainName]; ok {
			delete(attr, openstack.CredAttrDomainName)
			result.AuthCredentials[k] = cloud.NewCredential(v.AuthType(), attr)
		}
	}
	return result, nil
}
