// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

const (
	credAttrSDCUser    = "sdc-user"
	credAttrSDCKeyID   = "sdc-key-id"
	credAttrPrivateKey = "private-key"
	credAttrAlgorithm  = "algorithm"

	algorithmDefault = "rsa-sha256"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		// TODO(axw) we need a more appropriate name for this authentication
		//           type. ssh?
		cloud.UserPassAuthType: {{
			credAttrSDCUser, cloud.CredentialAttr{Description: "SmartDataCenter user ID"},
		}, {
			credAttrSDCKeyID, cloud.CredentialAttr{Description: "SmartDataCenter key ID"},
		}, {
			credAttrPrivateKey, cloud.CredentialAttr{
				Description: "Private key used to sign requests",
				Hidden:      true,
				FileAttr:    "private-key-path",
			},
		}, {
			credAttrAlgorithm, cloud.CredentialAttr{
				Description: "Algorithm used to generate the private key (default rsa-sha256)",
				Optional:    true,
				Options:     []interface{}{"rsa-sha256", "rsa-sha1", "rsa-sha224", "rsa-sha384", "rsa-sha512"},
			},
		}},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
