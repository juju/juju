// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/gce/internal/google"
)

const (
	credAttrPrivateKey  = "private-key"
	credAttrClientID    = "client-id"
	credAttrClientEmail = "client-email"
	credAttrProjectID   = "project-id"

	// The contents of the file for "jsonfile" auth-type.
	credAttrFile = "file"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.OAuth2AuthType: {{
			Name:           credAttrClientID,
			CredentialAttr: cloud.CredentialAttr{Description: "client ID"},
		}, {
			Name:           credAttrClientEmail,
			CredentialAttr: cloud.CredentialAttr{Description: "client e-mail address"},
		}, {
			Name: credAttrPrivateKey,
			CredentialAttr: cloud.CredentialAttr{
				Description: "client secret",
				Hidden:      true,
			},
		}, {
			Name:           credAttrProjectID,
			CredentialAttr: cloud.CredentialAttr{Description: "project ID"},
		}},
		cloud.JSONFileAuthType: {{
			Name: credAttrFile,
			CredentialAttr: cloud.CredentialAttr{
				Description: "path to the .json file containing a service account key for your project\nPath",
				FilePath:    true,
			},
		}},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	// Google recommends credentials in a json file:
	// 1. whose path is specified by the GOOGLE_APPLICATION_CREDENTIALS environment variable.
	// 2. whose location is known to the gcloud command-line tool.
	//   On *nix, this is $HOME/.config/gcloud/application_default_credentials.json.

	validatePath := func(possibleFilePath string) string {
		if possibleFilePath == "" {
			return ""
		}
		fi, err := os.Stat(possibleFilePath)
		if err != nil || fi.IsDir() {
			return ""
		}
		return possibleFilePath
	}

	possibleFilePath := validatePath(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	if possibleFilePath == "" {
		possibleFilePath = validatePath(wellKnownCredentialsFile())
	}
	if possibleFilePath == "" {
		return nil, errors.NotFoundf("gce credentials")
	}

	authFile, err := os.Open(possibleFilePath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer authFile.Close()

	parsedCred, err := parseJSONAuthFile(authFile)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid json credential file %s", possibleFilePath)
	}

	user, err := utils.LocalUsername()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cred := cloud.NewCredential(cloud.JSONFileAuthType, map[string]string{
		"file": possibleFilePath,
	})
	credName := parsedCred.Attributes()[credAttrClientEmail]
	if credName == "" {
		credName = parsedCred.Attributes()[credAttrClientID]
	}
	cred.Label = fmt.Sprintf("google credential %q", credName)
	return &cloud.CloudCredential{
		DefaultRegion: os.Getenv("CLOUDSDK_COMPUTE_REGION"),
		AuthCredentials: map[string]cloud.Credential{
			user: cred,
		}}, nil
}

func wellKnownCredentialsFile() string {
	const f = "application_default_credentials.json"
	return filepath.Join(utils.Home(), ".config", "gcloud", f)
}

// parseJSONAuthFile parses a file, and extracts the OAuth2 credentials within.
func parseJSONAuthFile(r io.Reader) (cloud.Credential, error) {
	creds, err := google.ParseJSONKey(r)
	if err != nil {
		return cloud.Credential{}, errors.Trace(err)
	}
	return cloud.NewCredential(cloud.OAuth2AuthType, map[string]string{
		credAttrProjectID:   creds.ProjectID,
		credAttrClientID:    creds.ClientID,
		credAttrClientEmail: creds.ClientEmail,
		credAttrPrivateKey:  string(creds.PrivateKey),
	}), nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
