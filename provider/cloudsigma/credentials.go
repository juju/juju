// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
)

type environProviderCredentials struct{}

const (
	credAttrUsername = "username"
	credAttrPassword = "password"
)

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: {{
			"username", cloud.CredentialAttr{
				Description: "account username",
			},
		}, {
			"password", cloud.CredentialAttr{
				Description: "account password",
				Hidden:      true,
			},
		}},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}
