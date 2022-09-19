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
		{
			Name: RBACLabelKeyName,
			CredentialAttr: cloud.CredentialAttr{
				Optional:    true,
				Description: "the unique ID key name of the rbac resources",
			},
		},
	},
	cloud.ClientCertificateAuthType: {
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

// LegacyCredentialsSchemas represents legacy credentials schemas that Juju used
// to output but still need to be supported to maintain working Kubernetes
// support. These types should be liberally allowed as input but not used as
// new output from Juju. This change was introduced by tlm in juju 2.9
var LegacyCredentialSchemas = map[cloud.AuthType]cloud.CredentialSchema{
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
		authType = cloud.ClientCertificateAuthType
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

func MigrateLegacyCredential(cred *cloud.Credential) (cloud.Credential, error) {
	switch cred.AuthType() {
	case cloud.OAuth2WithCertAuthType:
		return migrateOAuth2WithCertAuthType(cred)
	case cloud.CertificateAuthType:
		return migrateCertificateAuthType(cred)
	default:
		return cloud.Credential{}, errors.NotSupportedf(
			"migration for auth type %s", cred.AuthType())
	}
}

// CredentialToLegacy transform a valid k8s cloud credential to it's pre 2.9
// form. Kubernetes credentials were change in the Juju 2.9 release to fix bugs
// around the credential form.
func CredentialToLegacy(cred *cloud.Credential) (cloud.Credential, error) {
	attributes := map[string]string{}
	if val, exists := cred.Attributes()[RBACLabelKeyName]; exists {
		attributes[RBACLabelKeyName] = val
	}

	switch cred.AuthType() {
	case cloud.ClientCertificateAuthType:
		attributes[CredAttrClientCertificateData] = cred.Attributes()[CredAttrClientCertificateData]
		attributes[CredAttrClientKeyData] = cred.Attributes()[CredAttrClientKeyData]
		return cloud.NewNamedCredential(
			cred.Label,
			cloud.OAuth2WithCertAuthType,
			attributes,
			cred.Revoked,
		), nil
	case cloud.OAuth2AuthType:
		attributes[CredAttrToken] = cred.Attributes()[CredAttrToken]
		return cloud.NewNamedCredential(
			cred.Label,
			cloud.OAuth2WithCertAuthType,
			attributes,
			cred.Revoked,
		), nil
	default:
		return cloud.Credential{},
			errors.NotSupportedf(
				"credential with authtype %q cannot be converted to legacy kube credentials",
				cred.AuthType(),
			)
	}
}

func migrateCertificateAuthType(cred *cloud.Credential) (cloud.Credential, error) {
	attrs := cred.Attributes()
	newAttrs := map[string]string{}

	token, exists := attrs[CredAttrToken]
	if !exists {
		return cloud.Credential{}, errors.NotFoundf(
			"certificate oauth token during migration, expect key %s",
			CredAttrToken)
	}

	newAttrs[CredAttrToken] = token
	if _, exists := attrs[RBACLabelKeyName]; exists {
		newAttrs[RBACLabelKeyName] = attrs[RBACLabelKeyName]
	}

	return cloud.NewNamedCredential(
		cred.Label,
		cloud.OAuth2AuthType,
		newAttrs,
		false), nil
}

func migrateOAuth2WithCertAuthType(cred *cloud.Credential) (cloud.Credential, error) {
	attrs := cred.Attributes()
	newAttrs := map[string]string{}
	var authType cloud.AuthType

	_, clientCertExists := attrs[CredAttrClientCertificateData]
	_, clientCertKeyExists := attrs[CredAttrClientKeyData]
	if clientCertExists && clientCertKeyExists {
		authType = cloud.ClientCertificateAuthType
		newAttrs[CredAttrClientCertificateData] = attrs[CredAttrClientCertificateData]
		newAttrs[CredAttrClientKeyData] = attrs[CredAttrClientKeyData]
	} else if _, tokenExists := attrs[CredAttrToken]; tokenExists {
		authType = cloud.OAuth2AuthType
		newAttrs[CredAttrToken] = attrs[CredAttrToken]
	} else {
		return cloud.Credential{}, errors.NotValidf(
			"migrating oauth2cert must have either %s & %s attributes or %s attribute",
			CredAttrClientCertificateData,
			CredAttrClientKeyData,
			CredAttrToken)
	}

	return cloud.NewNamedCredential(
		cred.Label,
		authType,
		newAttrs,
		false), nil
}

// SupportedAuthTypes returns a slice of supported auth types that the Kubernetes
// caas provider supports.
func SupportedAuthTypes() cloud.AuthTypes {
	var ats cloud.AuthTypes
	for k := range LegacyCredentialSchemas {
		ats = append(ats, k)
	}
	ats = append(ats, SupportedNonLegacyAuthTypes()...)
	sort.Sort(ats)
	return ats
}

// SupportedNonLegacyAuthTypes returns a slice of supported auth types that
// Kubernetes caas provider supports with legacy auth types removed.
func SupportedNonLegacyAuthTypes() cloud.AuthTypes {
	var ats cloud.AuthTypes
	for k := range SupportedCredentialSchemas {
		ats = append(ats, k)
	}
	sort.Sort(ats)
	return ats
}

// UpdateCredentialWithToken patches the credential with the provided k8s secret token.
func UpdateCredentialWithToken(cred cloud.Credential, token string) (cloud.Credential, error) {
	if cred.AuthType() == "" {
		return cloud.Credential{}, errors.NotValidf("credential %q has empty auth type", cred.Label)
	}
	attributes := cred.Attributes()
	if attributes == nil {
		attributes = make(map[string]string)
	}
	attributes[CredAttrUsername] = ""
	attributes[CredAttrPassword] = ""
	attributes[CredAttrToken] = token
	return cloud.NewNamedCredential(cred.Label, cred.AuthType(), attributes, cred.Revoked), nil
}
