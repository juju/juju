// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/juju/cloud"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	// TODO (anastasiamac 2016-04-14) When/If this value changes,
	// verify that juju/juju/cloud/clouds.go#BuiltInClouds
	// with lxd type are up to-date.
	// TODO(wallyworld) update BuiltInClouds to match when we actually take notice of TLSAuthType
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.EmptyAuthType: {},
		cloud.CertificateAuthType: {
			{
				cfgClientCert, cloud.CredentialAttr{Description: "The client cert used for connecting to a LXD host machine."},
			}, {
				cfgClientKey, cloud.CredentialAttr{Description: "The client key used for connecting to a LXD host machine."},
			}, {
				cfgServerPEMCert, cloud.CredentialAttr{Description: "The certificate of the LXD server on the host machine."},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	return cloud.NewEmptyCloudCredential(), nil
}
