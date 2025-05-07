// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

var (
	_ = tc.Suite(&builtinSuite{})
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

func (s *builtinSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	var logger loggo.Logger
	s.runner = dummyRunner{CallMocker: testhelpers.NewCallMocker(logger)}
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

func (s *builtinSuite) TestGetLocalMicroK8sConfigFileDoesNotExists(c *tc.C) {
	s.runner.Call("LookPath", "microk8s").Returns("", nil)
	result, err := provider.GetLocalMicroK8sConfig(s.runner, func() (string, error) { return "non-exist-dir", nil })
	c.Assert(err, tc.ErrorMatches, `"non-exist-dir" does not exist: juju ".*" can only work with strictly confined microk8s`)
	c.Assert(result, tc.HasLen, 0)
}

func (s *builtinSuite) prepareKubeConfigFile(c *tc.C, content string) string {
	dir := c.MkDir()
	fileDir := filepath.Join(dir, "microk8s", "credentials")
	os.MkdirAll(fileDir, os.ModePerm)
	path := filepath.Join(fileDir, "client.config")
	err := os.WriteFile(path, []byte(content), 0660)
	c.Assert(err, tc.ErrorIsNil)
	return path
}

func (s *builtinSuite) TestAttemptMicroK8sCloud(c *tc.C) {
	s.runner.Call("LookPath", "microk8s").Returns("", nil)
	kubeconfigFile := s.prepareKubeConfigFile(c, microk8sConfig)

	k8sCloud, err := provider.AttemptMicroK8sCloud(s.runner, func() (string, error) { return kubeconfigFile, nil })
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(k8sCloud, tc.DeepEquals, cloud.Cloud{
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

func (s *builtinSuite) assertDecideKubeConfigDir(c *tc.C, isOfficial bool, clientConfigPath string) {
	s.PatchValue(&provider.CheckJujuOfficial, func(string) (semversion.Binary, bool, error) {
		return semversion.Binary{}, isOfficial, nil
	})
	s.PatchEnvironment("SNAP_DATA", "snap-data-dir")
	p, err := provider.DecideKubeConfigDir()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p, tc.DeepEquals, clientConfigPath)
}

func (s *builtinSuite) TestDecideKubeConfigDirOfficial(c *tc.C) {
	s.assertDecideKubeConfigDir(c, true, `snap-data-dir/microk8s/credentials/client.config`)
}

func (s *builtinSuite) TestDecideKubeConfigDirLocalBuild(c *tc.C) {
	s.assertDecideKubeConfigDir(c, false, `/var/snap/microk8s/current/credentials/client.config`)
}

func (s *builtinSuite) TestDecideKubeConfigDirNoJujud(c *tc.C) {
	s.PatchValue(&provider.CheckJujuOfficial, func(string) (semversion.Binary, bool, error) {
		return semversion.Binary{}, false, errors.NotFoundf("jujud")
	})
	s.PatchEnvironment("SNAP_DATA", "snap-data-dir")
	p, err := provider.DecideKubeConfigDir()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(p, tc.DeepEquals, `/var/snap/microk8s/current/credentials/client.config`)
}
