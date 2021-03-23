// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"io"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloud"
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
	runner dummyRunner

	kubeCloudParams provider.KubeCloudParams
}

func (s *builtinSuite) SetUpTest(c *gc.C) {
	var logger loggo.Logger
	s.runner = dummyRunner{CallMocker: testing.NewCallMocker(logger)}
	s.kubeCloudParams = provider.KubeCloudParams{
		ClusterName:   caas.MicroK8sClusterName,
		CloudName:     caas.K8sCloudMicrok8s,
		CredentialUID: caas.K8sCloudMicrok8s,
		CaasType:      provider.CAASProviderType,
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
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 1}, nil)

	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, gc.ErrorMatches, `microk8s not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.HasLen, 0)
}

func (s *builtinSuite) TestGetLocalMicroK8sConfigCallFails(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.config"}).Returns(&exec.ExecResponse{Code: 1, Stderr: []byte("cannot find config")}, nil)
	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, gc.ErrorMatches, `cannot find config`)
	c.Assert(result, gc.HasLen, 0)
}

func (s *builtinSuite) TestGetLocalMicroK8sConfig(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.config"}).Returns(&exec.ExecResponse{Code: 0, Stdout: []byte("a bunch of config")}, nil)

	result, err := provider.GetLocalMicroK8sConfig(s.runner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(result), gc.Equals, "a bunch of config")
}

func (s *builtinSuite) TestAttemptMicroK8sCloud(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.config"}).Returns(&exec.ExecResponse{Code: 0, Stdout: []byte(microk8sConfig)}, nil)

	k8sCloud, credential, credentialName, err := provider.AttemptMicroK8sCloud(s.runner, s.kubeCloudParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(k8sCloud, gc.DeepEquals, cloud.Cloud{
		Name:           caas.K8sCloudMicrok8s,
		Endpoint:       "http://1.1.1.1:8080",
		Type:           cloud.CloudTypeCAAS,
		AuthTypes:      []cloud.AuthType{cloud.CertificateAuthType},
		CACertificates: []string{"fakecadata1"},
		Description:    cloud.DefaultCloudDescription(cloud.CloudTypeCAAS),
		Regions: []cloud.Region{{
			Name: "localhost",
		}},
	})
	c.Assert(credential, gc.DeepEquals, cloud.NewNamedCredential(
		"microk8s", "certificate",
		map[string]string{
			"ClientCertificateData": `
-----BEGIN CERTIFICATE-----
MIIDBDCCAeygAwIBAgIJAPUHbpCysNxyMA0GCSqGSIb3DQEBCwUAMBcxFTATBgNV`[1:],
			"Token": "xfdfsdfsdsd",
		}, false,
	))
	c.Assert(credentialName, gc.Equals, "microk8s")
}

func (s *builtinSuite) TestAttemptMicroK8sCloudErrors(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "which microk8s.config"}).Returns(&exec.ExecResponse{Code: 1}, nil)
	k8sCloud, credential, credentialName, err := provider.AttemptMicroK8sCloud(s.runner, s.kubeCloudParams)
	c.Assert(err, gc.ErrorMatches, `microk8s not found`)
	c.Assert(k8sCloud, gc.DeepEquals, cloud.Cloud{})
	c.Assert(credential, gc.DeepEquals, cloud.Credential{})
	c.Assert(credentialName, gc.Equals, "")
}
