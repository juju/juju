// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/errors"
	"gopkg.in/amz.v3/aws"

	"github.com/juju/juju/cloud"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.AccessKeyAuthType: {
			"access-key": {
				Description: "The EC2 access key",
			},
			"secret-key": {
				Description: "The EC2 secret key",
				Hidden:      true,
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() ([]cloud.Credential, error) {
	// TODO(axw) check for credentials file as described at
	// http://blogs.aws.amazon.com/security/post/Tx3D6U6WSFGOK2H/A-New-and-Standardized-Way-to-Manage-Credentials-in-the-AWS-SDKs
	auth, err := aws.EnvAuth()
	if err != nil {
		return nil, errors.NewNotFound(err, "credentials not found")
	}
	accessKeyCredential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": auth.AccessKey,
			"secret-key": auth.SecretKey,
		},
	)
	return []cloud.Credential{accessKeyCredential}, nil
}
