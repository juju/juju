// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"io/ioutil"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	caascfg "github.com/juju/juju/caas/clientconfig"
	"github.com/juju/juju/cloud"
	"github.com/juju/testing"
)

type k8sConfigSuite struct {
	testing.IsolationSuite
	reader caascfg.K8SClientConfigReader
}

var _ = gc.Suite(&k8sConfigSuite{})

var (
	emptyConfig = `
apiVersion: v1
kind: Config
clusters: []
contexts: []
current-context: ""
preferences: {}
users: []
`

	singleConfig = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://1.1.1.1:8888
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
	multiConfig = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://1.1.1.1:8888
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

// writeTempKubeConfig writes yaml to a temp file and sets the
// KUBECONFIG environment variable so that the clientconfig code reads
// it instead of the default.
// The caller must close and remove the returned file.
func writeTempKubeConfig(c *gc.C, data string) *os.File {
	caasFile, err := ioutil.TempFile("", "caasFile")
	c.Assert(err, jc.ErrorIsNil)

	err = ioutil.WriteFile(caasFile.Name(), []byte(data), 0644)
	if err != nil {
		caasFile.Close()
		os.Remove(caasFile.Name())
		c.Fatal(err.Error())
	}
	os.Setenv("KUBECONFIG", caasFile.Name())

	return caasFile
}

func (s *k8sConfigSuite) TestGetEmptyConfig(c *gc.C) {
	tempFile := writeTempKubeConfig(c, emptyConfig)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	cfg, err := s.reader.GetClientConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals,
		&caascfg.ClientConfig{
			Type:           "kubernetes",
			Contexts:       map[string]caascfg.Context{},
			CurrentContext: "",
			Clouds:         map[string]caascfg.CloudConfig{},
			Credentials:    map[string]cloud.Credential{},
		})
}

func (s *k8sConfigSuite) TestGetSingleConfig(c *gc.C) {
	tempFile := writeTempKubeConfig(c, singleConfig)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	cfg, err := s.reader.GetClientConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals,
		&caascfg.ClientConfig{

			Type: "kubernetes",
			Contexts: map[string]caascfg.Context{
				"the-context": caascfg.Context{
					CloudName:      "the-cluster",
					CredentialName: "the-user"}},
			CurrentContext: "the-context",
			Clouds: map[string]caascfg.CloudConfig{
				"the-cluster": caascfg.CloudConfig{
					Endpoint:   "https://1.1.1.1:8888",
					Attributes: map[string]interface{}{"CAData": []uint8(nil)}}},
			Credentials: map[string]cloud.Credential{
				"the-user": cloud.NewCredential(
					cloud.UserPassAuthType,
					map[string]string{"Username": "theuser", "Password": "thepassword"})},
		})
}

func (s *k8sConfigSuite) TestGetMultiConfig(c *gc.C) {
	tempFile := writeTempKubeConfig(c, multiConfig)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	cfg, err := s.reader.GetClientConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals,
		&caascfg.ClientConfig{

			Type: "kubernetes",
			Contexts: map[string]caascfg.Context{
				"default-context": caascfg.Context{
					CloudName:      "default-cluster",
					CredentialName: "default-user"},
				"the-context": caascfg.Context{
					CloudName:      "the-cluster",
					CredentialName: "the-user"},
			},
			CurrentContext: "default-context",
			Clouds: map[string]caascfg.CloudConfig{
				"default-cluster": caascfg.CloudConfig{
					Endpoint:   "https://10.10.10.10:1010",
					Attributes: map[string]interface{}{"CAData": []uint8(nil)}},
				"the-cluster": caascfg.CloudConfig{
					Endpoint:   "https://1.1.1.1:8888",
					Attributes: map[string]interface{}{"CAData": []uint8(nil)}}},
			Credentials: map[string]cloud.Credential{
				"default-user": cloud.NewCredential(
					cloud.UserPassAuthType,
					map[string]string{"Username": "defaultuser", "Password": "defaultpassword"}),
				"the-user": cloud.NewCredential(
					cloud.CertificateAuthType,
					map[string]string{"ClientCertificateData": "A", "ClientKeyData": "B"}),
				"third-user": cloud.NewCredential(
					cloud.OAuth2AuthType,
					map[string]string{"Token": "atoken"}),
				"fourth-user": cloud.NewCredential(
					cloud.OAuth2WithCertAuthType,
					map[string]string{"ClientCertificateData": "A", "ClientKeyData": "B", "Token": "tokenwithcerttoken"}),
				"fifth-user": cloud.NewCredential(
					cloud.UserPassWithCertAuthType,
					map[string]string{"ClientCertificateData": "A", "ClientKeyData": "B", "Username": "fifth-user", "Password": "userpasscertpass"}),
			},
		})
}
