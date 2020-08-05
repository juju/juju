// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

const (
	CredAttrUsername              = "username"
	CredAttrPassword              = "password"
	CredAttrClientCertificateData = "ClientCertificateData"
	CredAttrClientKeyData         = "ClientKeyData"
	CredAttrToken                 = "Token"

	RBACLabelKeyName = "rbac-id"
)

var k8sCredentialSchemas = map[cloud.AuthType]cloud.CredentialSchema{
	cloud.UserPassAuthType: {
		{
			Name:           CredAttrUsername,
			CredentialAttr: cloud.CredentialAttr{Description: "The username to authenticate with."},
		}, {
			Name: CredAttrPassword,
			CredentialAttr: cloud.CredentialAttr{
				Description: "The password for the specified username.",
				Hidden:      true,
			},
		},
	},
	cloud.OAuth2WithCertAuthType: {
		{
			Name: CredAttrClientCertificateData,
			CredentialAttr: cloud.CredentialAttr{
				Description: "the kubernetes certificate data",
			},
		},
		{
			Name: CredAttrClientKeyData,
			CredentialAttr: cloud.CredentialAttr{
				Description: "the kubernetes private key data",
				Hidden:      true,
			},
		},
		{
			Name: CredAttrToken,
			CredentialAttr: cloud.CredentialAttr{
				Description: "the kubernetes token",
				Hidden:      true,
			},
		},
	},
	cloud.CertificateAuthType: {
		{
			Name: CredAttrClientCertificateData,
			CredentialAttr: cloud.CredentialAttr{
				Description: "the kubernetes certificate data",
			},
		},
		{
			Name: CredAttrToken,
			CredentialAttr: cloud.CredentialAttr{
				Description: "the kubernetes service account bearer token",
				Hidden:      true,
			},
		},
		{
			Name: RBACLabelKeyName,
			CredentialAttr: cloud.CredentialAttr{
				Optional:    true,
				Description: "the unique ID key name of the rbac resources",
			},
		},
	},
}

type environProviderCredentials struct {
	cmdRunner          CommandRunner
	builtinCloudGetter func(CommandRunner) (cloud.Cloud, cloud.Credential, string, error)
}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return k8sCredentialSchemas
}

func (environProviderCredentials) supportedAuthTypes() cloud.AuthTypes {
	var ats cloud.AuthTypes
	for k := range k8sCredentialSchemas {
		ats = append(ats, k)
	}
	return ats
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	clientConfigFunc, err := clientconfig.NewClientConfigReader(k8sconstants.CAASProviderType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	caasConfig, err := clientConfigFunc("", nil, "", "", nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(caasConfig.Contexts) == 0 {
		return nil, errors.NotFoundf("k8s cluster definitions")
	}

	defaultContext := caasConfig.Contexts[caasConfig.CurrentContext]
	result := &cloud.CloudCredential{
		AuthCredentials:   caasConfig.Credentials,
		DefaultCredential: defaultContext.CredentialName,
	}
	return result, nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}

// RegisterCredentials is part of the environs.ProviderCredentialsRegister interface.
func (p environProviderCredentials) RegisterCredentials(cld cloud.Cloud) (map[string]*cloud.CloudCredential, error) {
	cloudName := cld.Name
	if cloudName != caas.K8sCloudMicrok8s {
		return make(map[string]*cloud.CloudCredential), nil
	}
	_, cred, _, err := p.builtinCloudGetter(p.cmdRunner)

	if err != nil {
		return nil, errors.Trace(err)
	}

	return map[string]*cloud.CloudCredential{
		cloudName: {
			DefaultCredential: cloudName,
			AuthCredentials: map[string]cloud.Credential{
				cloudName: cred,
			},
		},
	}, nil
}
