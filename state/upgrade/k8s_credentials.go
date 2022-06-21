// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
)

// WARNING. Bear in mind that the code contained here is a
// pure replication of code already contained at
// caas/kubernetes/cloud/credential.go
// This is a quick workaround to remove dependencies between
// the state package and the kubernetes package. In this
// case, the following lines are only required by the upgrade
// operation run in upgrades/steps_29.go In particular, the
// UpdateLegacyKubernetesCloudCredentials in the state package
// is invoked, and this invokes a specific public function from
// the caas/kubernetes/cloud/credential.go The code formats and
// returns some credentials info and has no additional logic.

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
