// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

const (
	credAttrUser     = "user"
	credAttrPassword = "password"
	credAttrVMFolder = "vmfolder"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: {
			{
				Name: credAttrUser,
				CredentialAttr: cloud.CredentialAttr{
					Description: "The username to authenticate with.",
				},
			}, {
				Name: credAttrPassword,
				CredentialAttr: cloud.CredentialAttr{
					Description: "The password to authenticate with.",
					Hidden:      true,
				},
			}, {
				Name: credAttrVMFolder,
				CredentialAttr: cloud.CredentialAttr{
					Description: "The folder to add VMs from the model.",
					Optional:    true,
				},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
