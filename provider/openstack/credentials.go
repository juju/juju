// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/goose.v1/identity"
	"gopkg.in/ini.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

const (
	credAttrTenantName = "tenant-name"
	credAttrUserName   = "username"
	credAttrPassword   = "password"
	credAttrDomainName = "domain-name"
	credAttrAccessKey  = "access-key"
	credAttrSecretKey  = "secret-key"
)

type OpenstackCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (OpenstackCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: {
			{
				credAttrUserName, cloud.CredentialAttr{Description: "The username to authenticate with."},
			}, {
				credAttrPassword, cloud.CredentialAttr{
					Description: "The password for the specified username.",
					Hidden:      true,
				},
			}, {
				credAttrTenantName, cloud.CredentialAttr{Description: "The OpenStack tenant name."},
			}, {
				credAttrDomainName, cloud.CredentialAttr{
					Description: "The OpenStack domain name.",
					Optional:    true,
				},
			},
		},
		cloud.AccessKeyAuthType: {
			{
				credAttrAccessKey, cloud.CredentialAttr{Description: "The access key to authenticate with."},
			}, {
				credAttrSecretKey, cloud.CredentialAttr{
					Description: "The secret key to authenticate with.",
					Hidden:      true,
				},
			}, {
				credAttrTenantName, cloud.CredentialAttr{Description: "The OpenStack tenant name."},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (c OpenstackCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	result := cloud.CloudCredential{
		AuthCredentials: make(map[string]cloud.Credential),
	}

	// Try just using environment variables
	creds, user, region, err := c.detectCredential()
	if err == nil {
		result.DefaultRegion = region
		result.AuthCredentials[user] = *creds
	}

	// Now look for .novarc file in home dir.
	novarc := filepath.Join(utils.Home(), ".novarc")
	novaInfo, err := ini.LooseLoad(novarc)
	if err != nil {
		return nil, errors.Annotate(err, "loading novarc file")
	}
	stripExport := regexp.MustCompile(`(?i)^\s*export\s*`)
	keyValues := novaInfo.Section(ini.DEFAULT_SECTION).KeysHash()
	if len(keyValues) > 0 {
		for k, v := range keyValues {
			k = stripExport.ReplaceAllString(k, "")
			os.Setenv(k, v)
		}
		creds, user, region, err := c.detectCredential()
		if err == nil {
			result.DefaultRegion = region
			result.AuthCredentials[user] = *creds
		}
	}
	if len(result.AuthCredentials) == 0 {
		return nil, errors.NotFoundf("openstack credentials")
	}
	return &result, nil
}

func (c OpenstackCredentials) detectCredential() (*cloud.Credential, string, string, error) {
	creds := identity.CredentialsFromEnv()
	if creds.TenantName == "" {
		return nil, "", "", errors.NewNotFound(nil, "OS_TENANT_NAME environment variable not set")
	}
	if creds.User == "" {
		return nil, "", "", errors.NewNotFound(nil, "neither OS_USERNAME nor OS_ACCESS_KEY environment variable not set")
	}
	if creds.Secrets == "" {
		return nil, "", "", errors.NewNotFound(nil, "neither OS_PASSWORD nor OS_SECRET_KEY environment variable not set")
	}

	user, err := utils.LocalUsername()
	if err != nil {
		return nil, "", "", errors.Trace(err)
	}

	// If OS_USERNAME or NOVA_USERNAME is set, assume userpass.
	var credential cloud.Credential
	if os.Getenv("OS_USERNAME") != "" || os.Getenv("NOVA_USERNAME") != "" {
		user = creds.User
		credential = cloud.NewCredential(
			cloud.UserPassAuthType,
			map[string]string{
				credAttrUserName:   creds.User,
				credAttrPassword:   creds.Secrets,
				credAttrTenantName: creds.TenantName,
				credAttrDomainName: creds.DomainName,
			},
		)
	} else {
		credential = cloud.NewCredential(
			cloud.AccessKeyAuthType,
			map[string]string{
				credAttrAccessKey:  creds.User,
				credAttrSecretKey:  creds.Secrets,
				credAttrTenantName: creds.TenantName,
			},
		)
	}
	region := creds.Region
	if region == "" {
		region = "<unspecified>"
	}
	credential.Label = fmt.Sprintf("openstack region %q project %q user %q", region, creds.TenantName, user)
	return &credential, user, creds.Region, nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (OpenstackCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
