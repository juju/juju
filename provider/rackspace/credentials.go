// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"fmt"

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
func (Credentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: {
			{
				Name:           openstack.CredAttrUserName,
				CredentialAttr: cloud.CredentialAttr{Description: "The username to authenticate with."},
			}, {
				Name: openstack.CredAttrPassword,
				CredentialAttr: cloud.CredentialAttr{
					Description: "The password for the specified username.",
					Hidden:      true,
				},
			}, {
				Name:           openstack.CredAttrTenantName,
				CredentialAttr: cloud.CredentialAttr{Description: "The OpenStack tenant name."},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (c Credentials) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	result, err := c.OpenstackCredentials.DetectCredentials("")
	if err != nil {
		return nil, err
	}

	delete(result.AuthCredentials, string(cloud.AccessKeyAuthType))

	// delete domain name from creds, since rackspace doesn't use it, and it
	// confuses our code.
	for k, v := range result.AuthCredentials {
		attr := v.Attributes()
		delete(attr, openstack.CredAttrDomainName)
		one := cloud.NewCredential(v.AuthType(), attr)
		one.Label = fmt.Sprintf("rackspace credential for user %q", attr[openstack.CredAttrUserName])
		result.AuthCredentials[k] = one
	}
	return result, nil
}
