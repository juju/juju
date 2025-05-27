// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

type environProviderCredentials struct {
	cmdRunner               CommandRunner
	builtinCredentialGetter func(context.Context, CommandRunner) (cloud.Credential, error)
}

var _ environs.ProviderCredentials = (*environProviderCredentials)(nil)

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	schemas := make(map[cloud.AuthType]cloud.CredentialSchema)
	for k, v := range k8scloud.SupportedCredentialSchemas {
		schemas[k] = v
	}
	for k, v := range k8scloud.LegacyCredentialSchemas {
		schemas[k] = v
	}
	return schemas
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	clientConfigFunc, err := clientconfig.NewClientConfigReader(constants.CAASProviderType)
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
	cred, err := k8scloud.MigrateLegacyCredential(&args.Credential)
	if errors.Is(err, errors.NotSupported) {
		return &args.Credential, nil
	} else if err != nil {
		return &cred, errors.Annotatef(err, "migrating credential %s", args.Credential.Label)
	}
	return &cred, nil
}

// RegisterCredentials is part of the environs.ProviderCredentialsRegister interface.
func (p environProviderCredentials) RegisterCredentials(cld cloud.Cloud) (map[string]*cloud.CloudCredential, error) {
	cloudName := cld.Name
	if cloudName != k8s.K8sCloudMicrok8s {
		return registerCredentialsKubeConfig(context.TODO(), cld)
	}
	cred, err := p.builtinCredentialGetter(context.TODO(), p.cmdRunner)

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

func registerCredentialsKubeConfig(
	ctx context.Context,
	cld cloud.Cloud,
) (map[string]*cloud.CloudCredential, error) {
	k8sConfig, err := clientconfig.GetLocalKubeConfig()
	if err != nil {
		return make(map[string]*cloud.CloudCredential), errors.Annotate(err, "reading local kubeconf")
	}

	context, exists := k8sConfig.Contexts[cld.Name]
	if !exists {
		return make(map[string]*cloud.CloudCredential), nil
	}

	resolver := clientconfig.GetJujuAdminServiceAccountResolver(ctx, jujuclock.WallClock)
	conf, err := resolver(cld.Name, k8sConfig, cld.Name)
	if err != nil {
		return make(map[string]*cloud.CloudCredential), errors.Annotatef(
			err,
			"registering juju admin service account for cloud %q", cld.Name)
	}

	cred, err := k8scloud.CredentialFromKubeConfig(context.AuthInfo, conf)
	return map[string]*cloud.CloudCredential{
		cld.Name: {
			DefaultCredential: cld.Name,
			AuthCredentials: map[string]cloud.Credential{
				cld.Name: cred,
			},
		},
	}, err
}
