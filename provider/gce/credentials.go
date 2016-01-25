// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.OAuth2AuthType: {
			"client-id": {
				Description: "client ID",
			},
			"client-email": {
				Description: "client e-mail address",
			},
			"private-key": {
				Description: "client secret",
				Hidden:      true,
			},
			"project-id": {
				Description: "project ID",
			},
		},
		cloud.JSONFileAuthType: {
			"file": {
				Description: "path to the .json file containing your Google Compute Engine project credentials",
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() ([]cloud.Credential, error) {
	return nil, errors.NotFoundf("credentials")
}
