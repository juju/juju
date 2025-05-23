// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"context"
	"os"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/exec"

	"github.com/juju/juju/caas"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	k8sprovider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

func TestDetectCloudSuite(t *testing.T) {
	tc.Run(t, &detectCloudSuite{})
}

type detectCloudSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

type builtinCloudRet struct {
	cloud      jujucloud.Cloud
	credential jujucloud.Credential
	err        error
}

type dummyRunner struct {
	*testhelpers.CallMocker
}

func (d dummyRunner) RunCommands(run exec.RunParams) (*exec.ExecResponse, error) {
	results := d.MethodCall(d, "RunCommands", run)
	return results[0].(*exec.ExecResponse), testhelpers.TypeAssertError(results[1])
}

func (d dummyRunner) LookPath(file string) (string, error) {
	results := d.MethodCall(d, "LookPath", file)
	return results[0].(string), testhelpers.TypeAssertError(results[1])
}

func cloudGetterFunc(args builtinCloudRet) func(k8sprovider.CommandRunner) (jujucloud.Cloud, error) {
	return func(k8sprovider.CommandRunner) (jujucloud.Cloud, error) {
		return args.cloud, args.err
	}
}

func credentialGetterFunc(args builtinCloudRet) func(context.Context, k8sprovider.CommandRunner) (jujucloud.Credential, error) {
	return func(context.Context, k8sprovider.CommandRunner) (jujucloud.Credential, error) {
		return args.credential, args.err
	}
}

func (s *detectCloudSuite) getProvider(builtin builtinCloudRet) caas.ContainerEnvironProvider {
	return k8sprovider.NewProviderWithFakes(
		dummyRunner{},
		credentialGetterFunc(builtin),
		cloudGetterFunc(builtin),
		func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (k8sprovider.ClusterMetadataStorageChecker, error) {
			return &fakeK8sClusterMetadataChecker{}, nil
		},
	)
}

func (s *detectCloudSuite) TestDetectCloudsWithoutKubeConfig(c *tc.C) {
	c.Skip("This test is skipped because the cloud detector is not isolated from the test environment")
	err := os.Setenv("KUBECONFIG", "/tmp/doesnotexistkubeconfig.yaml")
	c.Assert(err, tc.ErrorIsNil)
	k8sCloud := jujucloud.Cloud{
		Name: "testingMicrok8s",
	}
	p := s.getProvider(builtinCloudRet{cloud: k8sCloud, err: nil})
	cloudDetector := p.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 1)
	c.Assert(clouds[0], tc.DeepEquals, k8sCloud)
}

func (s *detectCloudSuite) TestDetectCloudsMicroK8sNotFoundWithoutKubeConfig(c *tc.C) {
	c.Skip("This test is skipped because the cloud detector is not isolated from the test environment")
	err := os.Setenv("KUBECONFIG", "/tmp/doesnotexistkubeconfig.yaml")
	c.Assert(err, tc.ErrorIsNil)
	p := s.getProvider(builtinCloudRet{err: errors.NotFoundf("")})
	cloudDetector := p.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 0)
}

func (s *detectCloudSuite) TestDetectCloudsWithKubeConfig(c *tc.C) {
	c.Skip("This test is skipped because the cloud detector is not isolated from the test environment")
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

	file, err := os.CreateTemp("", "")
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()

	_, err = file.Write([]byte(kubeConfig))
	c.Assert(err, tc.ErrorIsNil)

	err = os.Setenv("KUBECONFIG", file.Name())
	c.Assert(err, tc.ErrorIsNil)

	k8sCloud := jujucloud.Cloud{
		Name: "testingMicrok8s",
	}
	p := s.getProvider(builtinCloudRet{cloud: k8sCloud, err: nil})
	cloudDetector := p.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 2)
	c.Assert(clouds[1], tc.DeepEquals, k8sCloud)
}

func (s *detectCloudSuite) TestDetectCloudMicrok8s(c *tc.C) {
	k8sCloud := jujucloud.Cloud{
		Name: "testingMicrok8s",
	}
	p := s.getProvider(builtinCloudRet{cloud: k8sCloud, err: nil})
	cloudDetector := p.(environs.CloudDetector)

	cloud, err := cloudDetector.DetectCloud("microk8s")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloud, tc.DeepEquals, k8sCloud)
}

func (s *detectCloudSuite) TestDetectCloudNotMicroK8s(c *tc.C) {
	p := s.getProvider(builtinCloudRet{})
	cloudDetector := p.(environs.CloudDetector)

	cloud, err := cloudDetector.DetectCloud("notmicrok8s")
	c.Assert(err, tc.ErrorMatches, `cloud notmicrok8s not found`)
	c.Assert(cloud, tc.DeepEquals, jujucloud.Cloud{})
}

func (s *detectCloudSuite) TestDetectCloudMicroK8sErrorsNotFound(c *tc.C) {
	p := s.getProvider(builtinCloudRet{err: errors.NotFoundf("")})
	cloudDetector := p.(environs.CloudDetector)

	cloud, err := cloudDetector.DetectCloud("notmicrok8s")
	c.Assert(err, tc.ErrorMatches, `cloud notmicrok8s not found`)
	c.Assert(cloud, tc.DeepEquals, jujucloud.Cloud{})
}
