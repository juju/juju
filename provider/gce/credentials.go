// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/provider/gce/google"
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

// parseJSONAuthFile parses the file with the given path, and extracts
// the OAuth2 credentials within.
func parseJSONAuthFile(filename string) (cloud.Credential, error) {
	authFile, err := os.Open(filename)
	if err != nil {
		return cloud.Credential{}, errors.Trace(err)
	}
	defer authFile.Close()
	creds, err := google.ParseJSONKey(authFile)
	if err != nil {
		return cloud.Credential{}, errors.Trace(err)
	}
	return cloud.NewCredential(cloud.OAuth2AuthType, map[string]string{
		"project-id":   creds.ProjectID,
		"client-id":    creds.ClientID,
		"client-email": creds.ClientEmail,
		"private-key":  string(creds.PrivateKey),
	}), nil
}
