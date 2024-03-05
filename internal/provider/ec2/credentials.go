// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"gopkg.in/ini.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

type environProviderCredentials struct{}

// AuthTypes returns all of the AuthTypes supported by the ec2 environ
// credentials provider.
func (e environProviderCredentials) AuthTypes() cloud.AuthTypes {
	credSchemas := e.CredentialSchemas()
	at := make(cloud.AuthTypes, 0, len(credSchemas))
	for k := range credSchemas {
		at = append(at, k)
	}
	return at
}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.AccessKeyAuthType: {
			{
				"access-key",
				cloud.CredentialAttr{
					Description: "The EC2 access key",
				},
			}, {
				"secret-key",
				cloud.CredentialAttr{
					Description: "The EC2 secret key",
					Hidden:      true,
				},
			},
		},
		cloud.InstanceRoleAuthType: {
			{
				"instance-profile-name",
				cloud.CredentialAttr{
					Description: "The AWS Instance Profile name",
				},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (e environProviderCredentials) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	dir := credentialsDir()
	credsFile := filepath.Join(dir, "credentials")
	credInfo, err := ini.LooseLoad(credsFile)
	if err != nil {
		return nil, errors.Annotate(err, "loading AWS credentials file")
	}
	credInfo.NameMapper = ini.TitleUnderscore

	// There's always a section called "DEFAULT" for top level items.
	if len(credInfo.Sections()) == 1 {
		// No standard AWS credentials so try environment variables.
		return e.detectEnvCredentials()
	}

	type accessKeyValues struct {
		AwsAccessKeyId     string
		AwsSecretAccessKey string
	}
	result := cloud.CloudCredential{
		AuthCredentials: make(map[string]cloud.Credential),
	}
	for _, credName := range credInfo.SectionStrings() {
		if credName == ini.DefaultSection {
			// No credentials at top level.
			continue
		}
		values := new(accessKeyValues)
		if err := credInfo.Section(credName).MapTo(values); err != nil {
			return nil, errors.Annotatef(err, "invalid credential attributes in section %q", credName)
		}
		// Basic validation check
		if values.AwsAccessKeyId == "" || values.AwsSecretAccessKey == "" {
			logger.Errorf("missing aws credential attributes in credentials file section %q", credName)
			continue
		}
		accessKeyCredential := cloud.NewCredential(
			cloud.AccessKeyAuthType,
			map[string]string{
				"access-key": values.AwsAccessKeyId,
				"secret-key": values.AwsSecretAccessKey,
			},
		)
		accessKeyCredential.Label = fmt.Sprintf("aws credential %q", credName)
		result.AuthCredentials[credName] = accessKeyCredential
	}

	// See if there's also a default region defined.
	configFile := filepath.Join(dir, "config")
	configInfo, err := ini.LooseLoad(configFile)
	if err != nil {
		return nil, errors.Annotate(err, "loading AWS config file")
	}
	result.DefaultRegion = configInfo.Section("default").Key("region").String()
	return &result, nil
}

func credentialsDir() string {
	return filepath.Join(utils.Home(), ".aws")
}

func (environProviderCredentials) detectEnvCredentials() (*cloud.CloudCredential, error) {
	cfg, err := config.NewEnvConfig()
	if err != nil || cfg.Credentials.AccessKeyID == "" {
		return nil, errors.NewNotFound(err, "credentials not found")
	}
	accessKeyCredential := cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"access-key": cfg.Credentials.AccessKeyID,
			"secret-key": cfg.Credentials.SecretAccessKey,
		},
	)
	user, err := utils.LocalUsername()
	if err != nil {
		return nil, errors.Trace(err)
	}
	accessKeyCredential.Label = fmt.Sprintf("aws credential %q", user)
	return &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			user: accessKeyCredential,
		}}, nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
