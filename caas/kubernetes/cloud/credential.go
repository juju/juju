// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"sort"

	"github.com/juju/errors"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/juju/juju/cloud"
)

const (
	// CredAttrUsername is the attribute key for username credentials
	CredAttrUsername = "username"
	// CredAttrPassword is the attribute key for password credentials
	CredAttrPassword = "password"
	// CredAttrClientCertificateData is the attribute key for client certificate credentials
	CredAttrClientCertificateData = "ClientCertificateData"
	// CredAttrClientKeyData is the attribute key for client certificate key credentials
	CredAttrClientKeyData = "ClientKeyData"
	// CredAttrToken is the attribute key for outh2 token credentials
	CredAttrToken = "Token"
	// RBACLabelKeyName key id for rbac credential labels
	RBACLabelKeyName = "rbac-id"
)

// SupportedCredentialSchemas holds the schemas that the kubernetes caas provider
// supports.
var SupportedCredentialSchemas = map[cloud.AuthType]cloud.CredentialSchema{
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
	cloud.OAuth2AuthType: {
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
			Name: CredAttrClientKeyData,
			CredentialAttr: cloud.CredentialAttr{
				Description: "the kubernetes certificate key",
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

// SupportedAuthTypes returns a slice of support auth types that the Kubernetes
// caas provider supports.
func SupportedAuthTypes() cloud.AuthTypes {
	var ats cloud.AuthTypes
	for k := range SupportedCredentialSchemas {
		ats = append(ats, k)
	}
	sort.Sort(ats)
	return ats
}

// CredentialFromAuthInfo will generate a Juju credential based on the supplied
// Kubernetes AuthInfo
func CredentialFromAuthInfo(
	authName string,
	authInfo *clientcmdapi.AuthInfo,
) (cloud.Credential, error) {
	attrs := map[string]string{}

	// TODO(axw) if the certificate/key are specified by path,
	// then we should just store the path in the credential,
	// and rely on credential finalization to read it at time
	// of use.

	clientCertData, err := dataOrFile(authInfo.ClientCertificateData, authInfo.ClientCertificate)
	if err != nil {
		return cloud.Credential{},
			errors.Annotatef(
				err,
				"getting authinfo %q client certificate",
				authName)
	}
	hasClientCert := len(clientCertData) > 0

	clientCertKeyData, err := dataOrFile(authInfo.ClientKeyData, authInfo.ClientKey)
	if err != nil {
		return cloud.Credential{},
			errors.Annotatef(
				err,
				"getting authinfo %q client certificate key",
				authName)
	}
	hasClientCertKey := len(clientCertKeyData) > 0

	token, err := stringOrFile(authInfo.Token, authInfo.TokenFile)
	if err != nil {
		return cloud.Credential{},
			errors.Annotatef(
				err,
				"getting authinfo %q token",
				authName)
	}
	hasToken := len(token) > 0

	var authType cloud.AuthType
	switch {
	case hasClientCert && hasClientCertKey:
		authType = cloud.CertificateAuthType
		attrs["ClientCertificateData"] = string(clientCertData)
		attrs["ClientKeyData"] = string(clientCertKeyData)
	case hasToken:
		authType = cloud.OAuth2AuthType
		attrs["Token"] = token
	case authInfo.Username != "":
		authType = cloud.UserPassAuthType
		attrs["username"] = authInfo.Username
		attrs["password"] = authInfo.Password
	default:
		return cloud.Credential{}, errors.NotSupportedf("configuration for %q", authName)
	}

	return cloud.NewNamedCredential(authName, authType, attrs, false), nil
}

// CredentialFromKubeConfig generates a Juju credential from the supplied
// Kubernetes config
func CredentialFromKubeConfig(
	authName string,
	config *clientcmdapi.Config,
) (cloud.Credential, error) {
	authInfo, exists := config.AuthInfos[authName]
	if !exists {
		return cloud.Credential{}, errors.NotFoundf("kubernetes config authinfo %q", authName)
	}
	c, err := CredentialFromAuthInfo(authName, authInfo)
	return c, err
}

// CredentialFromKubeConfigContext generate a Juju credential from the supplied
// Kubernetes config context.
func CredentialFromKubeConfigContext(
	ctxName string,
	config *clientcmdapi.Config,
) (cloud.Credential, error) {
	ctx, exists := config.Contexts[ctxName]
	if !exists {
		return cloud.Credential{}, errors.NotFoundf("could not find context %s", ctxName)
	}
	return CredentialFromKubeConfig(ctx.AuthInfo, config)
}
