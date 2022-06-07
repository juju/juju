// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	jujutesting "github.com/juju/juju/testing"
)

var (
	_ = gc.Suite(&builtinSuite{})
)

var microk8sConfig = `
apiVersion: v1
clusters:
- cluster:
    server: http://1.1.1.1:8080
  name: microk8s-cluster
contexts:
- context:
    cluster: microk8s-cluster
    user: admin
  name: microk8s
current-context: microk8s
kind: Config
preferences: {}
users:
- name: admin
  user:
    username: admin

`

type builtinSuite struct {
	jujutesting.BaseSuite
	runner dummyRunner

	kubeCloudParams provider.KubeCloudParams
}

func (s *builtinSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var logger loggo.Logger
	s.runner = dummyRunner{CallMocker: testing.NewCallMocker(logger)}
	s.kubeCloudParams = provider.KubeCloudParams{
		ClusterName:   k8s.MicroK8sClusterName,
		CloudName:     k8s.K8sCloudMicrok8s,
		CredentialUID: k8s.K8sCloudMicrok8s,
		CaasType:      constants.CAASProviderType,
		ClientConfigGetter: func(caasType string) (clientconfig.ClientConfigFunc, error) {
			return func(string, io.Reader, string, string, clientconfig.K8sCredentialResolver) (*clientconfig.ClientConfig, error) {
				return &clientconfig.ClientConfig{
					Type: "kubernetes",
					Contexts: map[string]clientconfig.Context{
						"microk8s": {CloudName: "microk8s", CredentialName: "microk8s"},
					},
					CurrentContext: "microk8s",
					Clouds: map[string]clientconfig.CloudConfig{
						"microk8s": {
							Endpoint: "http://1.1.1.1:8080",
							Attributes: map[string]interface{}{
								"CAData": "fakecadata1",
							},
						},
					},
					Credentials: map[string]cloud.Credential{
						"microk8s": cloud.NewNamedCredential(
							"microk8s", "certificate",
							map[string]string{
								"ClientCertificateData": `
-----BEGIN CERTIFICATE-----
MIIDBDCCAeygAwIBAgIJAPUHbpCysNxyMA0GCSqGSIb3DQEBCwUAMBcxFTATBgNV`[1:],
								"Token": "xfdfsdfsdsd",
							}, false,
						),
					},
				}, nil
			}, nil
		},
		Clock: testclock.NewClock(time.Time{}),
	}
}

func (s *builtinSuite) TestGetLocalMicroK8sConfigNotInstalled(c *gc.C) {
	s.runner.Call("LookPath", "microk8s").Returns("", errors.NotFoundf("microk8s"))
	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, gc.ErrorMatches, `microk8s not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.HasLen, 0)
}

func (s *builtinSuite) TestGetLocalMicroK8sConfigNoSNAP_DATA(c *gc.C) {
	s.runner.Call("LookPath", "microk8s").Returns("", nil)
	s.PatchEnvironment("SNAP_DATA", "")
	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, gc.ErrorMatches, `SNAP_DATA is empty: juju ".*" can only work with strict confined microk8s`)
	c.Assert(result, gc.HasLen, 0)
}

func (s *builtinSuite) TestGetLocalMicroK8sConfigFileDoesNotExists(c *gc.C) {
	s.runner.Call("LookPath", "microk8s").Returns("", nil)
	s.PatchEnvironment("SNAP_DATA", "non-exist-dir")
	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, gc.ErrorMatches, `"non-exist-dir/credentials/client.config" does not exist: juju ".*" can only work with strict confined microk8s`)
	c.Assert(result, gc.HasLen, 0)
}

func (s *builtinSuite) TestGetLocalMicroK8sConfigReadContentFile(c *gc.C) {
	s.runner.Call("LookPath", "microk8s").Returns("", nil)
	s.prepareKubeConfigFile(c, "client config file")
	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(result), gc.Equals, "client config file")
}

func (s *builtinSuite) prepareKubeConfigFile(c *gc.C, content string) {
	dir := c.MkDir()
	s.PatchEnvironment("SNAP_DATA", dir)
	os.MkdirAll(filepath.Join(dir, "credentials"), os.ModePerm)
	err := ioutil.WriteFile(filepath.Join(dir, "credentials", "client.config"), []byte(content), 0660)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *builtinSuite) TestAttemptMicroK8sCloud(c *gc.C) {
	s.runner.Call("LookPath", "microk8s").Returns("", nil)
	s.prepareKubeConfigFile(c, microk8sConfig)

	k8sCloud, err := provider.AttemptMicroK8sCloud(s.runner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(k8sCloud, gc.DeepEquals, cloud.Cloud{
		Name:     k8s.K8sCloudMicrok8s,
		Endpoint: "http://1.1.1.1:8080",
		Type:     cloud.CloudTypeKubernetes,
		AuthTypes: []cloud.AuthType{
			cloud.CertificateAuthType,
			cloud.ClientCertificateAuthType,
			cloud.OAuth2AuthType,
			cloud.OAuth2WithCertAuthType,
			cloud.UserPassAuthType,
		},
		CACertificates: []string{""},
		Description:    cloud.DefaultCloudDescription(cloud.CloudTypeKubernetes),
		Regions: []cloud.Region{{
			Name: "localhost",
		}},
	})
}

func (s *builtinSuite) TestAttemptMicroK8sCloudErrors(c *gc.C) {
	s.runner.Call("LookPath", "microk8s").Returns("", errors.NotFoundf("microk8s"))
	k8sCloud, err := provider.AttemptMicroK8sCloud(s.runner)
	c.Assert(err, gc.ErrorMatches, `microk8s not found`)
	c.Assert(k8sCloud, gc.DeepEquals, cloud.Cloud{})
}
