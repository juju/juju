// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		// TODO(axw) we need a more appropriate name for this authentication
		//           type. ssh?
		cloud.UserPassAuthType: {
			sdcUser: {
				Description: "SmartDataCenter user ID",
			},
			sdcKeyId: {
				Description: "SmartDataCenter key ID",
			},
			mantaUser: {
				Description: "Manta user ID",
			},
			mantaKeyId: {
				Description: "Manta key ID",
			},
			privateKey: {
				Description: "Private key used to sign requests",
				Hidden:      true,
				FileAttr:    privateKeyPath,
			},
			algorithm: {
				Description: "Algorithm used to generate the private key (default rsa-sha256)",
				Optional:    true,
				Options:     []interface{}{"rsa-sha256", "rsa-sha1", "rsa-sha224", "rsa-sha384", "rsa-sha512"},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}
