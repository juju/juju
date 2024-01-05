// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"context"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
	storagev1 "k8s.io/api/storage/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/provider/proxy"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/caas"
	"github.com/juju/juju/jujuclient"
	// To allow a maas cloud type to be parsed in the test data.
	_ "github.com/juju/juju/provider/maas"
	"github.com/juju/juju/rpc/params"
)

type updateCAASSuite struct {
	jujutesting.IsolationSuite
	dir                           string
	fakeCloudAPI                  *fakeUpdateCloudAPI
	fakeK8sClusterMetadataChecker *fakeK8sClusterMetadataChecker
	cloudMetadataStore            *fakeCloudMetadataStore
	clientStore                   *jujuclient.MemStore
}

var _ = gc.Suite(&updateCAASSuite{})

type fakeUpdateCloudAPI struct {
	*jujutesting.CallMocker
	caas.UpdateCloudAPI

	cloud       jujucloud.Cloud
	modelResult []params.UpdateCredentialModelResult
	errorResult *params.Error
}

func (api *fakeUpdateCloudAPI) Close() error {
	return nil
}

func (api *fakeUpdateCloudAPI) UpdateCloud(kloud cloud.Cloud) error {
	api.MethodCall(api, "UpdateCloud", kloud)
	return nil
}

func (api *fakeUpdateCloudAPI) Cloud(tag names.CloudTag) (cloud.Cloud, error) {
	api.MethodCall(api, "Cloud", tag)
	return api.cloud, nil
}

func (api *fakeUpdateCloudAPI) UpdateCloudsCredentials(cloudCredentials map[string]jujucloud.Credential, force bool) ([]params.UpdateCredentialResult, error) {
	api.MethodCall(api, "UpdateCloudsCredentials", cloudCredentials, force)
	var tag string
	for k := range cloudCredentials {
		tag = k
	}
	return []params.UpdateCredentialResult{
		{
			CredentialTag: tag,
			Models:        api.modelResult,
			Error:         api.errorResult,
		},
	}, nil
}

var cloudYaml = `
clouds:
  myk8s:
    type: kubernetes
    auth-types: [certificate]
    host-cloud-region: gce/us-east1
    endpoint: "https://6.6.6.6:8888"
    regions:
      us-east1:
        endpoint: "https://1.1.1.1:8888"
    ca-certificates:
    - fakecadata2
    config:
      operator-storage: operator-sc
      workload-storage: ""
  mrcloud1:
    type: maas
    description: Metal As A Service
  mrcloud2:
    type: kubernetes
    description: A Kubernetes Cluster
`[1:]

func (s *updateCAASSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.dir = c.MkDir()

	var logger loggo.Logger
	s.clientStore = NewMockClientStore()
	s.fakeCloudAPI = &fakeUpdateCloudAPI{
		CallMocker: jujutesting.NewCallMocker(logger),
	}
	s.cloudMetadataStore = &fakeCloudMetadataStore{CallMocker: jujutesting.NewCallMocker(logger)}

	defaultClusterMetadata := &k8s.ClusterMetadata{
		Cloud: "gce", Regions: set.NewStrings("us-east1"),
		OperatorStorageClass: &storagev1.StorageClass{
			ObjectMeta: meta.ObjectMeta{Name: "operator-sc"},
		},
	}
	s.fakeK8sClusterMetadataChecker = &fakeK8sClusterMetadataChecker{
		CallMocker: jujutesting.NewCallMocker(logger),
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(defaultClusterMetadata, nil)

	clouds, err := cloud.ParseCloudMetadata([]byte(cloudYaml))
	c.Assert(err, jc.ErrorIsNil)
	s.fakeCloudAPI.cloud = clouds["myk8s"]
	s.cloudMetadataStore.Call("ReadCloudData", "mycloud.yaml").Returns(cloudYaml, nil)
	s.cloudMetadataStore.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloud.Cloud{}, false, nil)
	s.cloudMetadataStore.Call("PersonalCloudMetadata").Returns(clouds, nil)
	s.cloudMetadataStore.Call("WritePersonalCloudMetadata", clouds).Returns(nil)
}

func (s *updateCAASSuite) makeCommand() cmd.Command {
	return caas.NewUpdateCAASCommandForTest(
		s.cloudMetadataStore,
		s.clientStore,
		func() (caas.UpdateCloudAPI, error) {
			return s.fakeCloudAPI, nil
		},
		func(_ context.Context, cloud jujucloud.Cloud, credential jujucloud.Credential) (k8s.ClusterMetadataChecker, error) {
			return s.fakeK8sClusterMetadataChecker, nil
		},
	)
}

func (s *updateCAASSuite) runCommand(c *gc.C, com cmd.Command, args ...string) (*cmd.Context, error) {
	ctx := cmdtesting.Context(c)
	c.Logf("run cmd with args: %v", args)
	if err := cmdtesting.InitCommand(com, args); err != nil {
		cmd.WriteError(ctx.Stderr, err)
		return ctx, err
	}
	return ctx, com.Run(ctx)
}

func (s *updateCAASSuite) TestUpdateExtraArg(c *gc.C) {
	command := s.makeCommand()
	_, err := s.runCommand(c, command, "k8sname", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *updateCAASSuite) TestMissingArgs(c *gc.C) {
	command := s.makeCommand()
	_, err := s.runCommand(c, command)
	c.Assert(err, gc.ErrorMatches, `k8s cloud name required`)
}

func (s *updateCAASSuite) TestCloudNotFound(c *gc.C) {
	command := s.makeCommand()
	_, err := s.runCommand(c, command, "foo")
	c.Assert(err, gc.ErrorMatches, `cloud foo not found`)
}

func (s *updateCAASSuite) assertUpdateCloudResult(
	c *gc.C, testRun func(),
	cloudName, cloudRegion, workloadStorage, operatorStorage string,
	t testData,
) {

	testRun()

	_, region, err := jujucloud.SplitHostCloudRegion(cloudRegion)
	c.Assert(err, jc.ErrorIsNil)
	s.fakeK8sClusterMetadataChecker.CheckNoCalls(c)
	expectedCloudToUpdate := cloud.Cloud{
		Name:             cloudName,
		HostCloudRegion:  cloudRegion,
		Type:             "kubernetes",
		Description:      "A Kubernetes Cluster",
		AuthTypes:        cloud.AuthTypes{"certificate"},
		Endpoint:         "https://6.6.6.6:8888",
		IdentityEndpoint: "",
		StorageEndpoint:  "",
		Config:           map[string]interface{}{"operator-storage": operatorStorage, "workload-storage": workloadStorage},
		RegionConfig:     cloud.RegionConfig(nil),
		CACertificates:   []string{"fakecadata2"},
	}
	if region != "" {
		expectedCloudToUpdate.Regions = []cloud.Region{{Name: region, Endpoint: "https://1.1.1.1:8888"}}
	}
	if !t.controller {
		s.fakeCloudAPI.CheckNoCalls(c)
	} else {
		s.fakeCloudAPI.CheckCall(c, 1, "UpdateCloud", expectedCloudToUpdate)
	}
	if t.client {
		s.cloudMetadataStore.CheckCall(c, 4, "WritePersonalCloudMetadata",
			map[string]cloud.Cloud{
				"mrcloud1": {
					Name:             "mrcloud1",
					Type:             "maas",
					Description:      "Metal As A Service",
					AuthTypes:        cloud.AuthTypes(nil),
					Endpoint:         "",
					IdentityEndpoint: "",
					StorageEndpoint:  "",
					Regions:          []cloud.Region(nil),
					Config:           map[string]interface{}(nil),
					RegionConfig:     cloud.RegionConfig(nil),
				},
				"mrcloud2": {
					Name:             "mrcloud2",
					Type:             "kubernetes",
					Description:      "A Kubernetes Cluster",
					AuthTypes:        cloud.AuthTypes(nil),
					Endpoint:         "",
					IdentityEndpoint: "",
					StorageEndpoint:  "",
					Regions:          []cloud.Region(nil),
					Config:           map[string]interface{}(nil),
					RegionConfig:     cloud.RegionConfig(nil),
				},
				"myk8s": expectedCloudToUpdate,
			},
		)
	}
}

func (s *updateCAASSuite) TestLocalOnly(c *gc.C) {
	s.assertUpdateCloudResult(c, func() {
		command := s.makeCommand()
		ctx, err := s.runCommand(c, command, "myk8s", "-f", "mycloud.yaml", "--client")
		c.Assert(err, jc.ErrorIsNil)
		expected := `k8s cloud "myk8s" updated on this client.`
		c.Assert(strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1), gc.Equals, expected)
	}, "myk8s", "gce/us-east1", "", "operator-sc", testData{client: true})
}

func (s *updateCAASSuite) TestInvalidControllerCloud(c *gc.C) {
	s.fakeCloudAPI.cloud = jujucloud.Cloud{Type: "maas"}
	command := s.makeCommand()
	ctx, err := s.runCommand(c, command, "myk8s", "-c", "foo")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1), gc.Equals,
		`The "myk8s" cloud on the controller is a "maas" cloud and not a "kubernetes" cloud.`)
}

func (s *updateCAASSuite) TestInvalidNewCloud(c *gc.C) {
	command := s.makeCommand()
	ctx, err := s.runCommand(c, command, "mrcloud1", "-c", "foo", "-f", "mycloud.yaml")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1), gc.Equals,
		`The "mrcloud1" cloud is a "maas" cloud and not a "kubernetes" cloud.`)
}

func (s *updateCAASSuite) TestControllerAndClient(c *gc.C) {
	s.assertUpdateCloudResult(c, func() {
		command := s.makeCommand()
		ctx, err := s.runCommand(c, command, "myk8s", "-f", "mycloud.yaml", "-c", "foo", "--client")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1), gc.Equals,
			`k8s cloud "myk8s" updated on this client.k8s cloud "myk8s" updated on controller "foo".`)
	}, "myk8s", "gce/us-east1", "", "operator-sc", testData{client: true, controller: true})
}

func (s *updateCAASSuite) TestBuiltinLocal(c *gc.C) {
	command := s.makeCommand()
	ctx, err := s.runCommand(c, command, "microk8s", "--client")
	c.Assert(err, jc.ErrorIsNil)
	expected := `k8s cloud "microk8s" updated on this client.`
	c.Assert(strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1), gc.Equals, expected)
	ctrl, ok := s.clientStore.Controllers["foo"]
	c.Assert(ok, jc.IsTrue)
	p, ok := ctrl.Proxy.Proxier.(*proxy.Proxier)
	c.Assert(ok, jc.IsTrue)
	y, err := yaml.Marshal(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.ReplaceAll(string(y), "\n", ""), gc.Matches, ".*api-host: 10.1.0.0:666.*")
}

func (s *updateCAASSuite) TestBuiltinWithFile(c *gc.C) {
	command := s.makeCommand()
	ctx, err := s.runCommand(c, command, "microk8s", "-f", "mycloud.yaml")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(strings.Replace(cmdtesting.Stderr(ctx), "\n", "", -1), gc.Equals,
		`"microk8s" is a built-in cloud and does not support specifying a cloud YAML file.`)
}

func (s *updateCAASSuite) TestBuiltinToController(c *gc.C) {
	var logger loggo.Logger
	microk8sClusterMetadata := &k8s.ClusterMetadata{
		Cloud: "microk8s",
	}
	s.fakeK8sClusterMetadataChecker = &fakeK8sClusterMetadataChecker{
		CallMocker: jujutesting.NewCallMocker(logger),
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(microk8sClusterMetadata, nil)

	command := s.makeCommand()
	_, err := s.runCommand(c, command, "microk8s", "-c", "foo")
	c.Assert(err, jc.ErrorIsNil)
	s.fakeK8sClusterMetadataChecker.CheckCall(c, 0, "GetClusterMetadata")
	expectedCloudToUpdate := cloud.Cloud{
		Name:            "microk8s",
		Endpoint:        "http://10.1.0.0:666",
		HostCloudRegion: "",
		Type:            "kubernetes",
		Description:     "",
		RegionConfig:    cloud.RegionConfig(nil),
		AuthTypes:       cloud.AuthTypes{"certificate"},
	}
	expectedCredToUpdate := map[string]cloud.Credential{
		"cloudcred-microk8s_foouser_default": cloud.NewNamedCredential("test", "", nil, false)}

	s.fakeCloudAPI.CheckCall(c, 1, "UpdateCloud", expectedCloudToUpdate)
	s.fakeCloudAPI.CheckCall(c, 2, "UpdateCloudsCredentials", expectedCredToUpdate, false)
}

func (s *updateCAASSuite) TestAffectedModels(c *gc.C) {
	var logger loggo.Logger
	s.fakeCloudAPI.modelResult = []params.UpdateCredentialModelResult{{
		ModelName: "test",
		ModelUUID: "uuid",
		Errors:    []params.ErrorResult{{Error: &params.Error{Message: "error"}}},
	}}
	microk8sClusterMetadata := &k8s.ClusterMetadata{
		Cloud: "microk8s",
	}
	s.fakeK8sClusterMetadataChecker = &fakeK8sClusterMetadataChecker{
		CallMocker: jujutesting.NewCallMocker(logger),
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(microk8sClusterMetadata, nil)

	command := s.makeCommand()
	ctx, err := s.runCommand(c, command, "microk8s", "-c", "foo")
	c.Assert(err, gc.DeepEquals, cmd.ErrSilent)

	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
k8s cloud "microk8s" updated on controller "foo".
Credential invalid for:
  test:
    error
Failed models may require a different credential.
Use ‘juju set-credential’ to change credential for these models before repeating this update.
`[1:])
}

func (s *updateCAASSuite) TestUpdateCredentialError(c *gc.C) {
	var logger loggo.Logger
	s.fakeCloudAPI.modelResult = []params.UpdateCredentialModelResult{{
		ModelName: "test",
		ModelUUID: "uuid",
		Errors:    []params.ErrorResult{{Error: &params.Error{Message: "error"}}},
	}}
	s.fakeCloudAPI.errorResult = &params.Error{Message: "some error"}
	microk8sClusterMetadata := &k8s.ClusterMetadata{
		Cloud: "microk8s",
	}
	s.fakeK8sClusterMetadataChecker = &fakeK8sClusterMetadataChecker{
		CallMocker: jujutesting.NewCallMocker(logger),
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(microk8sClusterMetadata, nil)

	command := s.makeCommand()
	ctx, err := s.runCommand(c, command, "microk8s", "-c", "foo")
	c.Assert(err, gc.DeepEquals, cmd.ErrSilent)

	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
k8s cloud "microk8s" updated on controller "foo".
Credential invalid for:
  test:
    error
Failed models may require a different credential.
Use ‘juju set-credential’ to change credential for these models before repeating this update.
`[1:])
	c.Assert(c.GetTestLog(), jc.Contains, `Controller credential "default" for user "foouser" for cloud "microk8s" on controller "foo" not updated: some error`)
}
