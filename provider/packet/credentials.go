package packet

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.AccessKeyAuthType: {
			{
				"project-id",
				cloud.CredentialAttr{
					Description: "Packet project ID",
				},
			}, {
				"api-token",
				cloud.CredentialAttr{
					Description: "Packet API token",
					Hidden:      true,
				},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (e environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	type accessKeyValues struct {
		ProjectID string
		AuthToken string
	}
	creds := accessKeyValues{}
	result := cloud.CloudCredential{
		AuthCredentials: make(map[string]cloud.Credential),
	}

	if val, present := os.LookupEnv("PACKET_AUTH_TOKEN"); present {
		creds.AuthToken = val
	} else {
		return nil, errors.Errorf("packet auth token not present")
	}

	if val, present := os.LookupEnv("PACKET_PROJECT_ID"); present {
		creds.ProjectID = val
	} else {
		return nil, errors.Errorf("packet project ID not present")
	}

	result.AuthCredentials["default"] = cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"project-id": creds.ProjectID,
			"api-token":  creds.AuthToken,
		},
	)

	return &result, nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
