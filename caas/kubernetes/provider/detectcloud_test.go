// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

var (
	_ = gc.Suite(&detectCloudSuite{})
)

type detectCloudSuite struct{}

type builtinCloudRet struct {
	cloud      jujucloud.Cloud
	credential jujucloud.Credential
	err        error
}

type dummyRunner struct {
	*testing.CallMocker
}

func (d dummyRunner) RunCommands(run exec.RunParams) (*exec.ExecResponse, error) {
	results := d.MethodCall(d, "RunCommands", run)
	return results[0].(*exec.ExecResponse), testing.TypeAssertError(results[1])
}

func (d dummyRunner) LookPath(file string) (string, error) {
	results := d.MethodCall(d, "LookPath", file)
	return results[0].(string), testing.TypeAssertError(results[1])
}

func cloudGetterFunc(args builtinCloudRet) func(provider.CommandRunner) (jujucloud.Cloud, error) {
	return func(provider.CommandRunner) (jujucloud.Cloud, error) {
		return args.cloud, args.err
	}
}

func credentialGetterFunc(args builtinCloudRet) func(provider.CommandRunner) (jujucloud.Credential, error) {
	return func(provider.CommandRunner) (jujucloud.Credential, error) {
		return args.credential, args.err
	}
}

func (s *detectCloudSuite) getProvider(builtin builtinCloudRet) caas.ContainerEnvironProvider {
	return provider.NewProviderWithFakes(
		dummyRunner{},
		credentialGetterFunc(builtin),
		cloudGetterFunc(builtin),
		func(environs.OpenParams) (caas.ClusterMetadataChecker, error) {
			return &fakeK8sClusterMetadataChecker{}, nil
		},
	)
}

func (s *detectCloudSuite) TestDetectCloudsWithoutKubeConfig(c *gc.C) {
	err := os.Setenv("KUBECONFIG", "/tmp/doesnotexistkubeconfig.yaml")
	c.Assert(err, jc.ErrorIsNil)
	k8sCloud := jujucloud.Cloud{
		Name: "testingMicrok8s",
	}
	p := s.getProvider(builtinCloudRet{cloud: k8sCloud, err: nil})
	cloudDetector := p.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	c.Assert(clouds[0], jc.DeepEquals, k8sCloud)
}

func (s *detectCloudSuite) TestDetectCloudsMicroK8sNotFoundWithoutKubeConfig(c *gc.C) {
	err := os.Setenv("KUBECONFIG", "/tmp/doesnotexistkubeconfig.yaml")
	c.Assert(err, jc.ErrorIsNil)
	p := s.getProvider(builtinCloudRet{err: errors.NotFoundf("")})
	cloudDetector := p.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 0)
}

func (s *detectCloudSuite) TestDetectCloudsWithKubeConfig(c *gc.C) {
	kubeConfig := `
apiVersion: v1
clusters:
- cluster:
    server: https://localhost:8443
  name: detect-example
contexts:
- context:
    cluster: detect-example
    namespace: default
    user: user1
  name: detect-example
users:
- name: user1
  user:
    username: test
    password: test
`

	file, err := ioutil.TempFile("", "")
	c.Assert(err, jc.ErrorIsNil)
	defer file.Close()

	_, err = file.Write([]byte(kubeConfig))
	c.Assert(err, jc.ErrorIsNil)

	err = os.Setenv("KUBECONFIG", file.Name())
	c.Assert(err, jc.ErrorIsNil)

	k8sCloud := jujucloud.Cloud{
		Name: "testingMicrok8s",
	}
	p := s.getProvider(builtinCloudRet{cloud: k8sCloud, err: nil})
	cloudDetector := p.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 2)
	c.Assert(clouds[1], jc.DeepEquals, k8sCloud)
}

func (s *detectCloudSuite) TestDetectCloudMicrok8s(c *gc.C) {
	k8sCloud := jujucloud.Cloud{
		Name: "testingMicrok8s",
	}
	p := s.getProvider(builtinCloudRet{cloud: k8sCloud, err: nil})
	cloudDetector := p.(environs.CloudDetector)

	cloud, err := cloudDetector.DetectCloud("microk8s")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, jc.DeepEquals, k8sCloud)
}

func (s *detectCloudSuite) TestDetectCloudNotMicroK8s(c *gc.C) {
	p := s.getProvider(builtinCloudRet{})
	cloudDetector := p.(environs.CloudDetector)

	cloud, err := cloudDetector.DetectCloud("notmicrok8s")
	c.Assert(err, gc.ErrorMatches, `cloud notmicrok8s not found`)
	c.Assert(cloud, jc.DeepEquals, jujucloud.Cloud{})
}

func (s *detectCloudSuite) TestDetectCloudMicroK8sErrorsNotFound(c *gc.C) {
	p := s.getProvider(builtinCloudRet{err: errors.NotFoundf("")})
	cloudDetector := p.(environs.CloudDetector)

	cloud, err := cloudDetector.DetectCloud("notmicrok8s")
	c.Assert(err, gc.ErrorMatches, `cloud notmicrok8s not found`)
	c.Assert(cloud, jc.DeepEquals, jujucloud.Cloud{})
}
