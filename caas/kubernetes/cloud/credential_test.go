// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"os"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
)

type credentialSuite struct {
}

func TestCredentialSuite(t *stdtesting.T) {
	tc.Run(t, &credentialSuite{})
}

func (s *credentialSuite) TestValidCredentials(c *tc.C) {
	tests := []struct {
		AuthInfo   *clientcmdapi.AuthInfo
		AuthType   cloud.AuthType
		Attributes map[string]string
		Name       string
		PreSetup   func(*clientcmdapi.AuthInfo) error
	}{
		{
			AuthInfo: &clientcmdapi.AuthInfo{
				ClientCertificateData: []byte("cert-data"),
				ClientKeyData:         []byte("cert-key-data"),
			},
			AuthType: cloud.ClientCertificateAuthType,
			Attributes: map[string]string{
				"ClientCertificateData": "cert-data",
				"ClientKeyData":         "cert-key-data",
			},
			Name: "client-cert-data",
		},
		{
			AuthInfo: &clientcmdapi.AuthInfo{},
			AuthType: cloud.ClientCertificateAuthType,
			Attributes: map[string]string{
				"ClientCertificateData": "cert-data",
				"ClientKeyData":         "cert-key-data",
			},
			Name: "client-cert-data-from-file",
			PreSetup: func(a *clientcmdapi.AuthInfo) error {
				certFile, err := os.CreateTemp("", "")
				if err != nil {
					return err
				}
				_, err = certFile.WriteString("cert-data")
				if err != nil {
					return err
				}
				certKeyFile, err := os.CreateTemp("", "")
				if err != nil {
					return err
				}
				_, err = certKeyFile.WriteString("cert-key-data")
				if err != nil {
					return err
				}

				a.ClientCertificate = certFile.Name()
				a.ClientKey = certKeyFile.Name()
				certFile.Close()
				certKeyFile.Close()
				return nil
			},
		},
		{
			AuthInfo: &clientcmdapi.AuthInfo{
				Token: "wef44t34f23",
			},
			AuthType: cloud.OAuth2AuthType,
			Attributes: map[string]string{
				"Token": "wef44t34f23",
			},
			Name: "token",
		},
		{
			AuthInfo: &clientcmdapi.AuthInfo{},
			AuthType: cloud.OAuth2AuthType,
			Attributes: map[string]string{
				"Token": "wef44t34f23",
			},
			Name: "token-from-file",
			PreSetup: func(a *clientcmdapi.AuthInfo) error {
				tokenFile, err := os.CreateTemp("", "")
				if err != nil {
					return err
				}
				_, err = tokenFile.WriteString("wef44t34f23")
				if err != nil {
					return err
				}

				a.TokenFile = tokenFile.Name()
				tokenFile.Close()
				return nil
			},
		},
		{
			AuthInfo: &clientcmdapi.AuthInfo{
				Username: "tlm",
				Password: "top-secret",
			},
			AuthType: cloud.UserPassAuthType,
			Attributes: map[string]string{
				"username": "tlm",
				"password": "top-secret",
			},
			Name: "username-password",
		},
	}

	for _, test := range tests {
		if test.PreSetup != nil {
			err := test.PreSetup(test.AuthInfo)
			c.Assert(err, tc.ErrorIsNil)
		}
		cred, err := k8scloud.CredentialFromAuthInfo(test.Name, test.AuthInfo)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(cred.AuthType(), tc.Equals, test.AuthType)
		c.Assert(cred.Attributes(), tc.DeepEquals, test.Attributes)
	}
}

func (s *credentialSuite) TestUnsupportedCredentials(c *tc.C) {
	authInfo := &clientcmdapi.AuthInfo{
		ClientKeyData: []byte("test"),
	}

	_, err := k8scloud.CredentialFromAuthInfo("unsupported", authInfo)
	c.Assert(err.Error(), tc.Equals, "configuration for \"unsupported\" not supported")
}

func (s *credentialSuite) TestUnsuportedCredentialMigration(c *tc.C) {
	cred := cloud.NewNamedCredential(
		"doesnotexist",
		cloud.ClientCertificateAuthType,
		map[string]string{},
		false)

	_, err := k8scloud.MigrateLegacyCredential(&cred)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *credentialSuite) TestCertificateAuthMigrationMissingToken(c *tc.C) {
	cred := cloud.NewNamedCredential(
		"missingtoken",
		cloud.CertificateAuthType,
		map[string]string{},
		false)

	_, err := k8scloud.MigrateLegacyCredential(&cred)
	c.Assert(err.Error(), tc.Equals, "certificate oauth token during migration, expect key Token not found")
}

func (s *credentialSuite) TestCertificateAuthMigration(c *tc.C) {
	cred := cloud.NewNamedCredential(
		"missingtoken",
		cloud.CertificateAuthType,
		map[string]string{
			"Token": "mytoken",
		},
		false)

	cred, err := k8scloud.MigrateLegacyCredential(&cred)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cred.AuthType(), tc.Equals, cloud.OAuth2AuthType)
	c.Assert(cred.Label, tc.Equals, "missingtoken")
	c.Assert(cred.Attributes(), tc.DeepEquals, map[string]string{
		"Token": "mytoken",
	})
}

func (s *credentialSuite) TestCertificateAuthMigrationRBACId(c *tc.C) {
	cred := cloud.NewNamedCredential(
		"missingtoken",
		cloud.CertificateAuthType,
		map[string]string{
			"Token":   "mytoken",
			"rbac-id": "id",
		},
		false)

	cred, err := k8scloud.MigrateLegacyCredential(&cred)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cred.AuthType(), tc.Equals, cloud.OAuth2AuthType)
	c.Assert(cred.Label, tc.Equals, "missingtoken")
	c.Assert(cred.Attributes(), tc.DeepEquals, map[string]string{
		"Token":   "mytoken",
		"rbac-id": "id",
	})
}

func (s *credentialSuite) TestOAuth2CertMigrationWithoutToken(c *tc.C) {
	cred := cloud.NewNamedCredential(
		"missingtoken",
		cloud.OAuth2WithCertAuthType,
		map[string]string{
			"ClientCertificateData": "data",
			"ClientKeyData":         "key",
		},
		false)

	cred, err := k8scloud.MigrateLegacyCredential(&cred)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cred.AuthType(), tc.Equals, cloud.ClientCertificateAuthType)
	c.Assert(cred.Label, tc.Equals, "missingtoken")
	c.Assert(cred.Attributes(), tc.DeepEquals, map[string]string{
		"ClientCertificateData": "data",
		"ClientKeyData":         "key",
	})
}

func (s *credentialSuite) TestOAuth2CertMigrationWithoutTokenCert(c *tc.C) {
	cred := cloud.NewNamedCredential(
		"missingtoken",
		cloud.OAuth2WithCertAuthType,
		map[string]string{
			"ClientCertificateData": "data",
		},
		false)

	_, err := k8scloud.MigrateLegacyCredential(&cred)
	c.Assert(err.Error(), tc.Equals, "migrating oauth2cert must have either ClientCertificateData & ClientKeyData attributes or Token attribute not valid")
}

func (s *credentialSuite) TestOAuth2CertMigrationWithToken(c *tc.C) {
	cred := cloud.NewNamedCredential(
		"missingtoken",
		cloud.OAuth2WithCertAuthType,
		map[string]string{
			"Token": "mytoken",
		},
		false)

	cred, err := k8scloud.MigrateLegacyCredential(&cred)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cred.AuthType(), tc.Equals, cloud.OAuth2AuthType)
	c.Assert(cred.Label, tc.Equals, "missingtoken")
	c.Assert(cred.Attributes(), tc.DeepEquals, map[string]string{
		"Token": "mytoken",
	})
}

func (s *credentialSuite) TestCredentialMigrationToLegacy(c *tc.C) {
	tests := []struct {
		PreCred  cloud.Credential
		PostCred cloud.Credential
	}{
		{
			PreCred: cloud.NewNamedCredential(
				"Test1",
				cloud.ClientCertificateAuthType,
				map[string]string{
					"ClientCertificateData": "AA==",
					"ClientKeyData":         "AA==",
				},
				false,
			),
			PostCred: cloud.NewNamedCredential(
				"Test1",
				cloud.OAuth2WithCertAuthType,
				map[string]string{
					"ClientCertificateData": "AA==",
					"ClientKeyData":         "AA==",
				},
				false,
			),
		},
		{
			PreCred: cloud.NewNamedCredential(
				"Test1",
				cloud.ClientCertificateAuthType,
				map[string]string{
					"ClientCertificateData": "AA==",
					"ClientKeyData":         "AA==",
					"rbac-id":               "foo-bar",
				},
				false,
			),
			PostCred: cloud.NewNamedCredential(
				"Test1",
				cloud.OAuth2WithCertAuthType,
				map[string]string{
					"ClientCertificateData": "AA==",
					"ClientKeyData":         "AA==",
					"rbac-id":               "foo-bar",
				},
				false,
			),
		},
		{
			PreCred: cloud.NewNamedCredential(
				"Test1",
				cloud.OAuth2AuthType,
				map[string]string{
					"Token":   "AA==",
					"rbac-id": "foo-bar",
				},
				false,
			),
			PostCred: cloud.NewNamedCredential(
				"Test1",
				cloud.OAuth2WithCertAuthType,
				map[string]string{
					"Token":   "AA==",
					"rbac-id": "foo-bar",
				},
				false,
			),
		},
		{
			PreCred: cloud.NewNamedCredential(
				"Test1",
				cloud.OAuth2AuthType,
				map[string]string{
					"ClientCertificateData": "AA==",
					"ClientKeyData":         "AA==",
					"Token":                 "AA==",
					"rbac-id":               "foo-bar",
				},
				false,
			),
			PostCred: cloud.NewNamedCredential(
				"Test1",
				cloud.OAuth2WithCertAuthType,
				map[string]string{
					"Token":   "AA==",
					"rbac-id": "foo-bar",
				},
				false,
			),
		},
		{
			PreCred: cloud.NewNamedCredential(
				"Test1",
				cloud.OAuth2AuthType,
				map[string]string{
					"Token":   "AA==",
					"rbac-id": "foo-bar",
				},
				true,
			),
			PostCred: cloud.NewNamedCredential(
				"Test1",
				cloud.OAuth2WithCertAuthType,
				map[string]string{
					"Token":   "AA==",
					"rbac-id": "foo-bar",
				},
				true,
			),
		},
	}

	for _, test := range tests {
		rval, err := k8scloud.CredentialToLegacy(&test.PreCred)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rval, tc.DeepEquals, test.PostCred)
	}
}

func (s *credentialSuite) TestPatchCloudCredentialForCloudSpec(c *tc.C) {
	credential := cloud.NewCredential(
		"auth-type",
		map[string]string{
			k8scloud.CredAttrUsername: "foo",
			k8scloud.CredAttrPassword: "pwd",
		},
	)
	updatedCredential, err := k8scloud.UpdateCredentialWithToken(credential, "token")
	c.Check(err, tc.ErrorIsNil)

	c.Check(updatedCredential.AuthType(), tc.Equals, cloud.AuthType("auth-type"))
	c.Check(updatedCredential.Attributes(), tc.DeepEquals, map[string]string{
		k8scloud.CredAttrUsername: "",
		k8scloud.CredAttrPassword: "",
		k8scloud.CredAttrToken:    "token",
	})

	credential = cloud.NewCredential("auth-type", nil)
	updatedCredential, err = k8scloud.UpdateCredentialWithToken(credential, "token")
	c.Check(err, tc.ErrorIsNil)

	c.Check(updatedCredential.AuthType(), tc.Equals, cloud.AuthType("auth-type"))
	c.Check(updatedCredential.Attributes(), tc.DeepEquals, map[string]string{
		k8scloud.CredAttrUsername: "",
		k8scloud.CredAttrPassword: "",
		k8scloud.CredAttrToken:    "token",
	})
}

func (s *credentialSuite) TestPatchCloudCredentialForCloudSpecFailedInValid(c *tc.C) {
	credential := cloud.NewNamedCredential(
		"foo", "", map[string]string{
			k8scloud.CredAttrUsername: "foo",
			k8scloud.CredAttrPassword: "pwd",
		}, false,
	)
	_, err := k8scloud.UpdateCredentialWithToken(credential, "token")
	c.Assert(err, tc.ErrorMatches, `credential "foo" has empty auth type not valid`)
}
