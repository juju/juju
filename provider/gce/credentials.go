// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/utils"

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
func (environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	// Google recommends credentials in a json file:
	// 1. whose path is specified by the GOOGLE_APPLICATION_CREDENTIALS environment variable.
	// 2. whose location is known to the gcloud command-line tool.
	//   On Windows, this is %APPDATA%/gcloud/application_default_credentials.json.
	//   On other systems, $HOME/.config/gcloud/application_default_credentials.json.

	validatePath := func(possbleFilePath string) string {
		if possbleFilePath == "" {
			return ""
		}
		fi, err := os.Stat(possbleFilePath)
		if err != nil || fi.IsDir() {
			return ""
		}
		return possbleFilePath
	}

	possbleFilePath := validatePath(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	if possbleFilePath == "" {
		possbleFilePath = validatePath(wellKnownCredentialsFile())
	}
	if possbleFilePath == "" {
		return nil, errors.NotFoundf("gce credentials")
	}
	parsedCred, err := parseJSONAuthFile(possbleFilePath)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid json credential file %s", possbleFilePath)
	}

	user, err := utils.LocalUsername()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cred := cloud.NewCredential(cloud.JSONFileAuthType, map[string]string{
		"file": possbleFilePath,
	})
	credName := parsedCred.Attributes()["client-email"]
	if credName == "" {
		credName = parsedCred.Attributes()["client-id"]
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
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "gcloud", f)
	}
	return filepath.Join(utils.Home(), ".config", "gcloud", f)
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
