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
		// TODO(axw) The current implementation expects the user to pass in
		//           private-key-path and the config validation reads that
		//           file to extract the private key. We should consider
		//           supporting specifying the key itself in the credentials.
		//           But the key itself is not a value I'd expect we'd prompt
		//           a user to enter interactively.
		//
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
			privateKeyPath: {
				Description: "Path to private key used to sign requests",
			},
			algorithm: {
				Description: "Algorithm used to generate the private key",
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() ([]cloud.Credential, error) {
	return nil, errors.NotFoundf("credentials")
}
