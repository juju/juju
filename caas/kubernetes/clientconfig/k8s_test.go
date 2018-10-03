// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	"github.com/juju/testing"
)

type k8sConfigSuite struct {
	testing.IsolationSuite
	dir string
}

var _ = gc.Suite(&k8sConfigSuite{})

var (
	prefixConfigYAML = `
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
`
	emptyConfigYAML = `
apiVersion: v1
kind: Config
clusters: []
contexts: []
current-context: ""
preferences: {}
users: []
`

	singleConfigYAML = prefixConfigYAML + `
- name: the-user
  user:
    password: thepassword
    username: theuser
`

	multiConfigYAML = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://1.1.1.1:8888
    certificate-authority-data: QQ==
  name: the-cluster
- cluster:
    server: https://10.10.10.10:1010
  name: default-cluster
contexts:
- context:
    cluster: the-cluster
    user: the-user
  name: the-context
- context:
    cluster: default-cluster
    user: default-user
  name: default-context
current-context: default-context
preferences: {}
users:
- name: the-user
  user:
    client-certificate-data: QQ==
    client-key-data: Qg==
- name: default-user
  user:
    password: defaultpassword
    username: defaultuser
- name: third-user
  user:
    token: "atoken"
- name: fourth-user
  user:
    client-certificate-data: QQ==
    client-key-data: Qg==
    token: "tokenwithcerttoken"
- name: fifth-user
  user:
    client-certificate-data: QQ==
    client-key-data: Qg==
    username: "fifth-user"
    password: "userpasscertpass"
`
)

func (s *k8sConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.dir = c.MkDir()
}

// writeTempKubeConfig writes yaml to a temp file and sets the
// KUBECONFIG environment variable so that the clientconfig code reads
// it instead of the default.
// The caller must close and remove the returned file.
func (s *k8sConfigSuite) writeTempKubeConfig(c *gc.C, filename string, data string) (*os.File, error) {
	fullpath := filepath.Join(s.dir, filename)
	err := ioutil.WriteFile(fullpath, []byte(data), 0644)
	if err != nil {
		c.Fatal(err.Error())
	}
	os.Setenv("KUBECONFIG", fullpath)

	f, err := os.Open(fullpath)
	return f, err
}

func (s *k8sConfigSuite) TestGetEmptyConfig(c *gc.C) {
	f, err := s.writeTempKubeConfig(c, "emptyConfig", emptyConfigYAML)
	defer f.Close()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := clientconfig.NewK8sClientConfig(f)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals,
		&clientconfig.ClientConfig{
			Type:           "kubernetes",
			Contexts:       map[string]clientconfig.Context{},
			CurrentContext: "",
			Clouds:         map[string]clientconfig.CloudConfig{},
			Credentials:    map[string]cloud.Credential{},
		})
}

func (s *k8sConfigSuite) TestGetSingleConfig(c *gc.C) {
	f, err := s.writeTempKubeConfig(c, "singleConfig", singleConfigYAML)
	defer f.Close()
	c.Assert(err, jc.ErrorIsNil)
	s.assertSingleConfig(c, f)
}

func (s *k8sConfigSuite) assertSingleConfig(c *gc.C, f *os.File) {
	cfg, err := clientconfig.NewK8sClientConfig(f)
	c.Assert(err, jc.ErrorIsNil)
	cred := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{"username": "theuser", "password": "thepassword"})
	cred.Label = `kubernetes credential "the-user"`
	c.Assert(cfg, jc.DeepEquals,
		&clientconfig.ClientConfig{

			Type: "kubernetes",
			Contexts: map[string]clientconfig.Context{
				"the-context": {
					CloudName:      "the-cluster",
					CredentialName: "the-user"}},
			CurrentContext: "the-context",
			Clouds: map[string]clientconfig.CloudConfig{
				"the-cluster": {
					Endpoint:   "https://1.1.1.1:8888",
					Attributes: map[string]interface{}{"CAData": "A"}}},
			Credentials: map[string]cloud.Credential{
				"the-user": cred,
			},
		})
}

func (s *k8sConfigSuite) TestConfigErrors(c *gc.C) {
	for i, test := range []struct {
		title          string
		userConfigYAML string
		errMatch       string
	}{
		{
			title: "notSupportedAuthProviderTypeConfig",
			userConfigYAML: `
- name: the-user
  user:
    auth-provider:
      config:
        cmd-args: config config-helper --format=json
        cmd-path: /usr/lib/google-cloud-sdk/bin/gcloud
        expiry-key: '{.credential.token_expiry}'
        token-key: '{.credential.access_token}'
      name: gcp
`,
			errMatch: `failed to read credentials from kubernetes config: configuration for "the-user" not supported`,
		},
		{
			title: "tokenWithUsernameInvalidConfig",
			userConfigYAML: `
- name: the-user
  user:
    password: defaultpassword
    username: defaultuser
    token: nonEmptyToken
`,
			errMatch: `failed to read credentials from kubernetes config: AuthInfo: "the-user" with both Token and User/Pass not valid`,
		},
		{
			title: "emptyClientKeyDataInvalidConfig",
			userConfigYAML: `
- name: the-user
  user:
    client-certificate-data: QQ==
    client-key-data:
`,
			errMatch: `failed to read credentials from kubernetes config: empty ClientKeyData for \"the-user\" with auth type \"certificate\" not valid`,
		},
	} {
		c.Logf("test %d", i)
		f, err := s.writeTempKubeConfig(c, test.title, prefixConfigYAML+test.userConfigYAML)
		defer f.Close()
		c.Assert(err, jc.ErrorIsNil)
		_, err = clientconfig.NewK8sClientConfig(f)
		c.Check(err, gc.ErrorMatches, test.errMatch)
	}
}

func (s *k8sConfigSuite) TestGetMultiConfig(c *gc.C) {
	f, err := s.writeTempKubeConfig(c, "multiConfig", multiConfigYAML)
	defer f.Close()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := clientconfig.NewK8sClientConfig(f)
	c.Assert(err, jc.ErrorIsNil)
	firstCred := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{"username": "defaultuser", "password": "defaultpassword"})
	firstCred.Label = `kubernetes credential "default-user"`
	secondCred := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{"ClientCertificateData": "A", "ClientKeyData": "B"})
	secondCred.Label = `kubernetes credential "the-user"`
	thirdCred := cloud.NewCredential(
		cloud.OAuth2AuthType,
		map[string]string{"Token": "atoken"})
	thirdCred.Label = `kubernetes credential "third-user"`
	fourthCred := cloud.NewCredential(
		cloud.OAuth2WithCertAuthType,
		map[string]string{"ClientCertificateData": "A", "ClientKeyData": "B", "Token": "tokenwithcerttoken"})
	fourthCred.Label = `kubernetes credential "fourth-user"`
	fifthCred := cloud.NewCredential(
		cloud.UserPassWithCertAuthType,
		map[string]string{"ClientCertificateData": "A", "ClientKeyData": "B", "username": "fifth-user", "password": "userpasscertpass"})
	fifthCred.Label = `kubernetes credential "fifth-user"`
	c.Assert(cfg, jc.DeepEquals,
		&clientconfig.ClientConfig{
			Type: "kubernetes",
			Contexts: map[string]clientconfig.Context{
				"default-context": {
					CloudName:      "default-cluster",
					CredentialName: "default-user"},
				"the-context": {
					CloudName:      "the-cluster",
					CredentialName: "the-user"},
			},
			CurrentContext: "default-context",
			Clouds: map[string]clientconfig.CloudConfig{
				"default-cluster": {
					Endpoint:   "https://10.10.10.10:1010",
					Attributes: map[string]interface{}{"CAData": ""}},
				"the-cluster": {
					Endpoint:   "https://1.1.1.1:8888",
					Attributes: map[string]interface{}{"CAData": "A"}}},
			Credentials: map[string]cloud.Credential{
				"default-user": firstCred,
				"the-user":     secondCred,
				"third-user":   thirdCred,
				"fourth-user":  fourthCred,
				"fifth-user":   fifthCred,
			},
		})
}

// TestGetSingleConfigReadsFilePaths checks that we handle config
// with certificate/key file paths the same as we do those with
// the data inline.
func (s *k8sConfigSuite) TestGetSingleConfigReadsFilePaths(c *gc.C) {

	singleConfig, err := clientcmd.Load([]byte(singleConfigYAML))
	c.Assert(err, jc.ErrorIsNil)

	tempdir := c.MkDir()
	divert := func(name string, data *[]byte, path *string) {
		*path = filepath.Join(tempdir, name)
		err := ioutil.WriteFile(*path, *data, 0644)
		c.Assert(err, jc.ErrorIsNil)
		*data = nil
	}

	for name, cluster := range singleConfig.Clusters {
		divert(
			"cluster-"+name+".ca",
			&cluster.CertificateAuthorityData,
			&cluster.CertificateAuthority,
		)
	}

	for name, authInfo := range singleConfig.AuthInfos {
		divert(
			"auth-"+name+".cert",
			&authInfo.ClientCertificateData,
			&authInfo.ClientCertificate,
		)
		divert(
			"auth-"+name+".key",
			&authInfo.ClientKeyData,
			&authInfo.ClientKey,
		)
	}

	singleConfigWithPathsYAML, err := clientcmd.Write(*singleConfig)
	c.Assert(err, jc.ErrorIsNil)
	f, err := s.writeTempKubeConfig(c, "singleConfigWithPaths", string(singleConfigWithPathsYAML))
	defer f.Close()
	c.Assert(err, jc.ErrorIsNil)
	s.assertSingleConfig(c, f)
}
