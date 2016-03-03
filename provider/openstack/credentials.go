// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"os"

	"github.com/juju/errors"
	"gopkg.in/goose.v1/identity"

	"github.com/juju/juju/cloud"
)

type OpenstackCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (OpenstackCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: {
			"username": {
				Description: "The username to authenticate with.",
			},
			"password": {
				Description: "The password for the specified username.",
				Hidden:      true,
			},
			"tenant-name": {
				Description: "The OpenStack tenant name.",
			},
		},
		cloud.AccessKeyAuthType: {
			"access-key": {
				Description: "The access key to authenticate with.",
			},
			"secret-key": {
				Description: "The secret key to authenticate with.",
				Hidden:      true,
			},
			"tenant-name": {
				Description: "The OpenStack tenant name.",
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (OpenstackCredentials) DetectCredentials() ([]cloud.Credential, error) {
	creds := identity.CredentialsFromEnv()
	if creds.TenantName == "" {
		return nil, errors.NewNotFound(nil, "OS_TENANT_NAME environment variable not set")
	}
	if creds.User == "" {
		return nil, errors.NewNotFound(nil, "neither OS_USERNAME nor OS_ACCESS_KEY environment variable not set")
	}
	if creds.Secrets == "" {
		return nil, errors.NewNotFound(nil, "neither OS_PASSWORD nor OS_SECRET_KEY environment variable not set")
	}
	// If OS_USERNAME or NOVA_USERNAME is set, assume userpass.
	var credential cloud.Credential
	if os.Getenv("OS_USERNAME") != "" || os.Getenv("NOVA_USERNAME") != "" {
		credential = cloud.NewCredential(
			cloud.UserPassAuthType,
			map[string]string{
				"username":    creds.User,
				"password":    creds.Secrets,
				"tenant-name": creds.TenantName,
			},
		)
	} else {
		credential = cloud.NewCredential(
			cloud.AccessKeyAuthType,
			map[string]string{
				"access-key":  creds.User,
				"secret-key":  creds.Secrets,
				"tenant-name": creds.TenantName,
			},
		)
	}
	return []cloud.Credential{credential}, nil
}
