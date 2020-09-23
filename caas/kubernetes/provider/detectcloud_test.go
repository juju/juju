// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

var (
	_ = gc.Suite(&detectCloudSuite{})
)

type detectCloudSuite struct{}

type builtinCloudRet struct {
	cloud          cloud.Cloud
	credential     jujucloud.Credential
	credentialName string
	err            error
}

type dummyRunner struct {
	*testing.CallMocker
}

func (d dummyRunner) RunCommands(run exec.RunParams) (*exec.ExecResponse, error) {
	results := d.MethodCall(d, "RunCommands", run)
	return results[0].(*exec.ExecResponse), testing.TypeAssertError(results[1])
}

func getterFunc(args builtinCloudRet) func(provider.CommandRunner) (cloud.Cloud, jujucloud.Credential, string, error) {
	return func(provider.CommandRunner) (cloud.Cloud, jujucloud.Credential, string, error) {
		return args.cloud, args.credential, args.credentialName, args.err
	}
}

func (s *detectCloudSuite) getProvider(builtin builtinCloudRet) caas.ContainerEnvironProvider {
	return provider.NewProviderWithFakes(
		dummyRunner{},
		getterFunc(builtin),
		func(environs.OpenParams) (caas.ClusterMetadataChecker, error) {
			return &fakeK8sClusterMetadataChecker{}, nil
		},
	)
}

func (s *detectCloudSuite) TestDetectClouds(c *gc.C) {
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

func (s *detectCloudSuite) TestDetectCloudsMicroK8sNotFound(c *gc.C) {
	p := s.getProvider(builtinCloudRet{err: errors.NotFoundf("")})
	cloudDetector := p.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 0)
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
