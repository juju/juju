// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

const (
	credAttrMAASOAuth = "maas-oauth"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.OAuth1AuthType: {{
			credAttrMAASOAuth, cloud.CredentialAttr{
				Description: "OAuth/API-key credentials for MAAS",
				Hidden:      true,
			},
		}},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	// MAAS stores credentials in a json file: ~/.maasrc
	// {"Server": "http://<ip>/MAAS", "OAuth": "<key>"}
	maasrc := filepath.Join(utils.Home(), ".maasrc")
	fileBytes, err := os.ReadFile(maasrc)
	if os.IsNotExist(err) {
		return nil, errors.NotFoundf("maas credentials")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	details := make(map[string]interface{})
	err = json.Unmarshal(fileBytes, &details)
	if err != nil {
		return nil, errors.Trace(err)
	}
	oauthKey := details["OAuth"]
	if oauthKey == "" {
		return nil, errors.New("MAAS credentials require a value for OAuth token")
	}
	cred := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{
		credAttrMAASOAuth: fmt.Sprintf("%v", oauthKey),
	})
	server, ok := details["Server"]
	if server == "" || !ok {
		server = "unspecified server"
	}
	cred.Label = fmt.Sprintf("MAAS credential for %s", server)

	return &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"default": cred,
		},
	}, nil
}

func parseOAuthToken(cred cloud.Credential) (string, error) {
	oauth := cred.Attributes()[credAttrMAASOAuth]
	if strings.Count(oauth, ":") != 2 {
		return "", errMalformedMaasOAuth
	}
	return oauth, nil
}

var errMalformedMaasOAuth = errors.New("malformed maas-oauth (3 items separated by colons)")

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
