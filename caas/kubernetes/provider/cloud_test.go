// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

var (
	_ = gc.Suite(&cloudSuite{})
)

var microk8sStatusEnabled = `
microk8s:
  running: true
addons:
  jaeger: disabled
  fluentd: disabled
  gpu: disabled
  storage: enabled
  registry: disabled
  ingress: disabled
  dns: enabled
  metrics-server: disabled
  prometheus: disabled
  istio: disabled
  dashboard: disabled
`

var microk8sStatusStorageDisabled = `
microk8s:
  running: true
addons:
  jaeger: disabled
  fluentd: disabled
  gpu: disabled
  storage: disabled
  registry: disabled
  ingress: disabled
  dns: enabled
  metrics-server: disabled
  prometheus: disabled
  istio: disabled
  dashboard: disabled
`
var microk8sStatusDNSDisabled = `
microk8s:
  running: true
addons:
  jaeger: disabled
  fluentd: disabled
  gpu: disabled
  storage: enabled
  registry: disabled
  ingress: disabled
  dns: disabled
  metrics-server: disabled
  prometheus: disabled
  istio: disabled
  dashboard: disabled
`

type cloudSuite struct {
	fakeBroker fakeK8sClusterMetadataChecker
	runner     dummyRunner
}

var defaultK8sCloud = jujucloud.Cloud{
	Name:           k8s.K8sCloudMicrok8s,
	Endpoint:       "http://1.1.1.1:8080",
	Type:           cloud.CloudTypeKubernetes,
	AuthTypes:      []cloud.AuthType{cloud.UserPassAuthType},
	CACertificates: []string{""},
	SkipTLSVerify:  true,
}

var defaultClusterMetadata = &k8s.ClusterMetadata{
	Cloud:                k8s.K8sCloudMicrok8s,
	Regions:              set.NewStrings(k8s.Microk8sRegion),
	OperatorStorageClass: &k8s.StorageProvisioner{Name: "operator-sc"},
}

func getDefaultCredential() cloud.Credential {
	defaultCredential := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{"username": "admin", "password": ""})
	defaultCredential.Label = "kubernetes credential \"admin\""
	return defaultCredential
}

func (s *cloudSuite) SetUpTest(c *gc.C) {
	var logger loggo.Logger
	s.fakeBroker = fakeK8sClusterMetadataChecker{CallMocker: testing.NewCallMocker(logger)}
	s.runner = dummyRunner{CallMocker: testing.NewCallMocker(logger)}
}

func (s *cloudSuite) TestFinalizeCloudMicrok8s(c *gc.C) {
	p := s.getProvider()
	cloudFinalizer := p.(environs.CloudFinalizer)

	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: `id -nG "$(whoami)" | grep -qw "root\|microk8s"`}).Returns(
		&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.status --wait-ready --timeout 15 --yaml"}).Returns(
		&exec.ExecResponse{Code: 0, Stdout: []byte(microk8sStatusEnabled)}, nil)

	var ctx mockContext
	cloud, err := cloudFinalizer.FinalizeCloud(&ctx, defaultK8sCloud)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, jc.DeepEquals, jujucloud.Cloud{
		Name:            k8s.K8sCloudMicrok8s,
		Type:            jujucloud.CloudTypeKubernetes,
		AuthTypes:       []jujucloud.AuthType{jujucloud.UserPassAuthType},
		CACertificates:  []string{""},
		SkipTLSVerify:   true,
		Endpoint:        "http://1.1.1.1:8080",
		HostCloudRegion: fmt.Sprintf("%s/%s", k8s.K8sCloudMicrok8s, k8s.Microk8sRegion),
		Config:          map[string]interface{}{"operator-storage": "operator-sc", "workload-storage": ""},
		Regions:         []jujucloud.Region{{Name: k8s.Microk8sRegion, Endpoint: "http://1.1.1.1:8080"}},
	})
}

func (s *cloudSuite) TestFinalizeCloudMicrok8sAlreadyStorage(c *gc.C) {
	preparedCloud := jujucloud.Cloud{
		Name:            k8s.K8sCloudMicrok8s,
		Type:            jujucloud.CloudTypeKubernetes,
		AuthTypes:       []jujucloud.AuthType{jujucloud.UserPassAuthType},
		CACertificates:  []string{""},
		Endpoint:        "http://1.1.1.1:8080",
		HostCloudRegion: fmt.Sprintf("%s/%s", k8s.K8sCloudMicrok8s, k8s.Microk8sRegion),
		Config:          map[string]interface{}{"operator-storage": "something-else", "workload-storage": ""},
		Regions:         []jujucloud.Region{{Name: k8s.Microk8sRegion, Endpoint: "http://1.1.1.1:8080"}},
	}

	p := s.getProvider()
	cloudFinalizer := p.(environs.CloudFinalizer)

	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: `id -nG "$(whoami)" | grep -qw "root\|microk8s"`}).Returns(
		&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.status --wait-ready --timeout 15 --yaml"}).Returns(
		&exec.ExecResponse{Code: 0, Stdout: []byte(microk8sStatusEnabled)}, nil)

	var ctx mockContext
	cloud, err := cloudFinalizer.FinalizeCloud(&ctx, preparedCloud)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, jc.DeepEquals, jujucloud.Cloud{
		Name:            k8s.K8sCloudMicrok8s,
		Type:            jujucloud.CloudTypeKubernetes,
		AuthTypes:       []jujucloud.AuthType{jujucloud.UserPassAuthType},
		CACertificates:  []string{""},
		Endpoint:        "http://1.1.1.1:8080",
		HostCloudRegion: fmt.Sprintf("%s/%s", k8s.K8sCloudMicrok8s, k8s.Microk8sRegion),
		Config:          map[string]interface{}{"operator-storage": "something-else", "workload-storage": ""},
		Regions:         []jujucloud.Region{{Name: k8s.Microk8sRegion, Endpoint: "http://1.1.1.1:8080"}},
	})
}

func (s *cloudSuite) getProvider() caas.ContainerEnvironProvider {
	s.fakeBroker.Call("GetClusterMetadata").Returns(defaultClusterMetadata, nil)
	s.fakeBroker.Call("CheckDefaultWorkloadStorage").Returns(nil)
	ret := builtinCloudRet{cloud: defaultK8sCloud, credential: getDefaultCredential(), err: nil}
	return provider.NewProviderWithFakes(
		s.runner,
		credentialGetterFunc(ret),
		cloudGetterFunc(ret),
		func(environs.OpenParams) (k8s.ClusterMetadataChecker, error) { return &s.fakeBroker, nil },
	)
}

func (s *cloudSuite) TestEnsureMicroK8sSuitableSuccess(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: `id -nG "$(whoami)" | grep -qw "root\|microk8s"`}).Returns(
		&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.status --wait-ready --timeout 15 --yaml"}).Returns(
		&exec.ExecResponse{Code: 0, Stdout: []byte(microk8sStatusEnabled)}, nil)
	c.Assert(provider.EnsureMicroK8sSuitable(s.runner), jc.ErrorIsNil)
}

func (s *cloudSuite) TestEnsureMicroK8sSuitableStorageDisabled(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: `id -nG "$(whoami)" | grep -qw "root\|microk8s"`}).Returns(
		&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.status --wait-ready --timeout 15 --yaml"}).Returns(
		&exec.ExecResponse{Code: 0, Stdout: []byte(microk8sStatusStorageDisabled)}, nil)
	c.Assert(provider.EnsureMicroK8sSuitable(s.runner), gc.ErrorMatches, `required addons not enabled for microk8s, run 'microk8s enable storage'`)
}

func (s *cloudSuite) TestEnsureMicroK8sSuitableDNSDisabled(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: `id -nG "$(whoami)" | grep -qw "root\|microk8s"`}).Returns(
		&exec.ExecResponse{Code: 0}, nil)
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: "microk8s.status --wait-ready --timeout 15 --yaml"}).Returns(
		&exec.ExecResponse{Code: 0, Stdout: []byte(microk8sStatusDNSDisabled)}, nil)
	c.Assert(provider.EnsureMicroK8sSuitable(s.runner), gc.ErrorMatches, `required addons not enabled for microk8s, run 'microk8s enable dns'`)
}

func (s *cloudSuite) TestEnsureMicroK8sSuitableNotInGroup(c *gc.C) {
	s.runner.Call(
		"RunCommands",
		exec.RunParams{Commands: `id -nG "$(whoami)" | grep -qw "root\|microk8s"`}).Returns(
		&exec.ExecResponse{Code: 1}, nil)
	err := provider.EnsureMicroK8sSuitable(s.runner)
	c.Assert(err, gc.NotNil)
	c.Assert(strings.Replace(err.Error(), "\n", "", -1),
		gc.Matches, `The microk8s user group is created during the microk8s snap installation.*`)
}

type mockContext struct {
	testing.Stub
}

func (c *mockContext) Verbosef(f string, args ...interface{}) {
	c.MethodCall(c, "Verbosef", f, args)
}

type fakeK8sClusterMetadataChecker struct {
	*testing.CallMocker
	k8s.ClusterMetadataChecker
}

func (api *fakeK8sClusterMetadataChecker) GetClusterMetadata(storageClass string) (result *k8s.ClusterMetadata, err error) {
	results := api.MethodCall(api, "GetClusterMetadata")
	return results[0].(*k8s.ClusterMetadata), testing.TypeAssertError(results[1])
}

func (api *fakeK8sClusterMetadataChecker) CheckDefaultWorkloadStorage(cluster string, storageProvisioner *k8s.StorageProvisioner) error {
	results := api.MethodCall(api, "CheckDefaultWorkloadStorage")
	return testing.TypeAssertError(results[0])
}

func (api *fakeK8sClusterMetadataChecker) EnsureStorageProvisioner(cfg k8s.StorageProvisioner) (*k8s.StorageProvisioner, bool, error) {
	results := api.MethodCall(api, "EnsureStorageProvisioner")
	return results[0].(*k8s.StorageProvisioner), false, testing.TypeAssertError(results[1])
}
