// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import "github.com/juju/juju/cloud"

type environProviderCredentials struct{}

func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{cloud.EmptyAuthType: {}}
}

func (environProviderCredentials) DetectCredentials() ([]cloud.Credential, error) {
	emptyCredential := cloud.NewEmptyCredential()
	return []cloud.Credential{emptyCredential}, nil
}
