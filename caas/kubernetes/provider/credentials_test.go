// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"path/filepath"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/utils/v4"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
)

type credentialsSuite struct {
	testing.FakeHomeSuite
	provider environs.EnvironProvider
}

var _ = tc.Suite(&credentialsSuite{})

func (s *credentialsSuite) SetUpTest(c *tc.C) {
	s.FakeHomeSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("kubernetes")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	envtesting.AssertProviderAuthTypes(c, s.provider, "userpass", "oauth2", "clientcertificate", "oauth2withcert", "certificate")
}

func (s *credentialsSuite) TestCredentialsValid(c *tc.C) {
	envtesting.AssertProviderCredentialsValid(c, s.provider, "userpass", map[string]string{
		"username": "fred",
		"password": "secret",
	})
}

func (s *credentialsSuite) TestHiddenAttributes(c *tc.C) {
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "userpass", "password")
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "oauth2", "Token")
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "clientcertificate", "ClientKeyData")
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "oauth2withcert", "ClientKeyData", "Token")
	envtesting.AssertProviderCredentialsAttributesHidden(c, s.provider, "certificate", "Token")
}

var singleConfigYAML = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://1.1.1.1:8888
    certificate-authority-data: QQ==
  name: the-cluster
contexts:
- context:
    cluster: the-cluster
    user: the-user
  name: the-context
current-context: the-context
preferences: {}
users:
- name: the-user
  user:
    password: thepassword
    username: theuser
`

func (s *credentialsSuite) TestDetectCredentials(c *tc.C) {
	kubeConfig := filepath.Join(utils.Home(), "config")
	s.PatchEnvironment("KUBECONFIG", kubeConfig)
	s.Home.AddFiles(c, testing.TestFile{
		Name: "config",
		Data: singleConfigYAML,
	})
	creds, err := s.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(creds.DefaultRegion, tc.Equals, "")
	expected := cloud.NewNamedCredential(
		"the-user", cloud.UserPassAuthType, map[string]string{
			"username": "theuser",
			"password": "thepassword",
		}, false,
	)
	c.Assert(creds.AuthCredentials["the-user"], tc.DeepEquals, expected)
}

func (s *credentialsSuite) TestRegisterCredentialsNotMicrok8s(c *tc.C) {
	p := provider.NewProviderCredentials(credentialGetterFunc(builtinCloudRet{}))
	credentials, err := p.RegisterCredentials(cloud.Cloud{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials, tc.HasLen, 0)
}

func (s *credentialsSuite) TestRegisterCredentialsMicrok8s(c *tc.C) {
	p := provider.NewProviderCredentials(
		credentialGetterFunc(
			builtinCloudRet{
				cloud:      defaultK8sCloud,
				credential: getDefaultCredential(),
				err:        nil,
			},
		),
	)
	credentials, err := p.RegisterCredentials(defaultK8sCloud)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials, tc.HasLen, 1)
	c.Assert(credentials[k8s.K8sCloudMicrok8s], tc.DeepEquals, &cloud.CloudCredential{
		DefaultCredential: k8s.K8sCloudMicrok8s,
		AuthCredentials: map[string]cloud.Credential{
			k8s.K8sCloudMicrok8s: getDefaultCredential(),
		},
	})
}
