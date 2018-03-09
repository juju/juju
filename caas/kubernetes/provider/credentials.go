// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"
	"github.com/juju/juju/caas/kubernetes/clientconfig"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

const (
	CredAttrUsername = "username"
	CredAttrPassword = "password"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: {
			{
				CredAttrUsername, cloud.CredentialAttr{Description: "The username to authenticate with."},
			}, {
				CredAttrPassword, cloud.CredentialAttr{
					Description: "The password for the specified username.",
					Hidden:      true,
				},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	clientConfigFunc, err := clientconfig.NewClientConfigReader("kubernetes")
	if err != nil {
		return nil, errors.Trace(err)
	}
	caasConfig, err := clientConfigFunc()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(caasConfig.Contexts) == 0 {
		return nil, errors.NotFoundf("k8s cluster definitions")
	}

	defaultContext := caasConfig.Contexts[caasConfig.CurrentContext]
	result := &cloud.CloudCredential{
		AuthCredentials:   caasConfig.Credentials,
		DefaultCredential: defaultContext.CredentialName,
	}
	return result, nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
