// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	jujucaas "github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/caas"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

type addCAASSuite struct {
	jujutesting.IsolationSuite
	dir                           string
	initialCloudMap               map[string]cloud.Cloud
	fakeCloudAPI                  *fakeAddCloudAPI
	fakeK8sClusterMetadataChecker *fakeK8sClusterMetadataChecker
	cloudMetadataStore            *fakeCloudMetadataStore
	fakeK8SConfigFunc             *clientconfig.ClientConfigFunc
}

var _ = gc.Suite(&addCAASSuite{})

var kubeConfigStr = `
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
- name: the-user
  user:
    password: thepassword
    username: theuser
`

type fakeCloudMetadataStore struct {
	*jujutesting.CallMocker
}

func (f *fakeCloudMetadataStore) ParseCloudMetadataFile(path string) (map[string]cloud.Cloud, error) {
	results := f.MethodCall(f, "ParseCloudMetadataFile", path)
	return results[0].(map[string]cloud.Cloud), jujutesting.TypeAssertError(results[1])
}

func (f *fakeCloudMetadataStore) ParseOneCloud(data []byte) (cloud.Cloud, error) {
	results := f.MethodCall(f, "ParseOneCloud", data)
	return results[0].(cloud.Cloud), jujutesting.TypeAssertError(results[1])
}

func (f *fakeCloudMetadataStore) PublicCloudMetadata(searchPaths ...string) (result map[string]cloud.Cloud, fallbackUsed bool, _ error) {
	results := f.MethodCall(f, "PublicCloudMetadata", searchPaths)
	return results[0].(map[string]cloud.Cloud), results[1].(bool), jujutesting.TypeAssertError(results[2])
}

func (f *fakeCloudMetadataStore) PersonalCloudMetadata() (map[string]cloud.Cloud, error) {
	results := f.MethodCall(f, "PersonalCloudMetadata")
	return results[0].(map[string]cloud.Cloud), jujutesting.TypeAssertError(results[1])
}

func (f *fakeCloudMetadataStore) WritePersonalCloudMetadata(cloudsMap map[string]cloud.Cloud) error {
	results := f.MethodCall(f, "WritePersonalCloudMetadata", cloudsMap)
	return jujutesting.TypeAssertError(results[0])
}

type fakeAddCloudAPI struct {
	*jujutesting.CallMocker
	caas.AddCloudAPI
	isCloudRegionRequired bool
	authTypes             []cloud.AuthType
	credentials           []names.CloudCredentialTag
}

func (api *fakeAddCloudAPI) Close() error {
	return nil
}

func (api *fakeAddCloudAPI) AddCloud(kloud cloud.Cloud) error {
	api.MethodCall(api, "AddCloud", kloud)
	if kloud.HostCloudRegion == "" && api.isCloudRegionRequired {
		return params.Error{Code: params.CodeCloudRegionRequired}
	}
	return nil
}

func (api *fakeAddCloudAPI) AddCredential(tag string, credential cloud.Credential) error {
	return nil
}

type fakeK8sClusterMetadataChecker struct {
	*jujutesting.CallMocker
	jujucaas.ClusterMetadataChecker
}

func (api *fakeK8sClusterMetadataChecker) GetClusterMetadata(storageClass string) (result *jujucaas.ClusterMetadata, err error) {
	results := api.MethodCall(api, "GetClusterMetadata")
	return results[0].(*jujucaas.ClusterMetadata), jujutesting.TypeAssertError(results[1])
}

func (api *fakeK8sClusterMetadataChecker) CheckDefaultWorkloadStorage(cluster string, storageProvisioner *jujucaas.StorageProvisioner) error {
	results := api.MethodCall(api, "CheckDefaultWorkloadStorage")
	return jujutesting.TypeAssertError(results[0])
}

func (api *fakeK8sClusterMetadataChecker) EnsureStorageProvisioner(cfg jujucaas.StorageProvisioner) (*jujucaas.StorageProvisioner, error) {
	results := api.MethodCall(api, "EnsureStorageProvisioner", cfg)
	return results[0].(*jujucaas.StorageProvisioner), jujutesting.TypeAssertError(results[1])
}

func fakeNewK8sClientConfig(_ io.Reader, contextName, clusterName string, _ clientconfig.K8sCredentialResolver) (*clientconfig.ClientConfig, error) {
	cCfg := &clientconfig.ClientConfig{
		CurrentContext: "key1",
	}
	contexts := map[string]clientconfig.Context{
		"key1": {
			CloudName:      "mrcloud1",
			CredentialName: "credname1",
		},
		"key2": {
			CloudName:      "mrcloud2",
			CredentialName: "credname2",
		},
	}
	clouds := map[string]clientconfig.CloudConfig{
		"mrcloud1": {
			Endpoint: "fakeendpoint1",
			Attributes: map[string]interface{}{
				"CAData": "fakecadata1",
			},
		},
		"mrcloud2": {
			Endpoint: "fakeendpoint2",
			Attributes: map[string]interface{}{
				"CAData": "fakecadata2",
			},
		},
	}

	var context clientconfig.Context
	if contextName == "" {
		contextName = cCfg.CurrentContext
	}
	if clusterName != "" {
		var err error
		context, contextName, err = func() (clientconfig.Context, string, error) {
			for contextName, context := range contexts {
				if clusterName == context.CloudName {
					return context, contextName, nil
				}
			}
			return clientconfig.Context{}, "", errors.NotFoundf("context for cluster name %q", clusterName)
		}()
		if err != nil {
			return nil, err
		}
	} else {
		context = contexts[contextName]
	}
	cCfg.Contexts = map[string]clientconfig.Context{contextName: context}
	cCfg.Clouds = map[string]clientconfig.CloudConfig{context.CloudName: clouds[context.CloudName]}
	return cCfg, nil
}

func fakeEmptyNewK8sClientConfig(io.Reader, string, string, clientconfig.K8sCredentialResolver) (*clientconfig.ClientConfig, error) {
	return &clientconfig.ClientConfig{}, nil
}

func (s *addCAASSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.dir = c.MkDir()

	var logger loggo.Logger
	s.fakeCloudAPI = &fakeAddCloudAPI{
		CallMocker: jujutesting.NewCallMocker(logger),
		authTypes: []cloud.AuthType{
			cloud.EmptyAuthType,
			cloud.AccessKeyAuthType,
		},
		credentials: []names.CloudCredentialTag{
			names.NewCloudCredentialTag("cloud/admin/default"),
			names.NewCloudCredentialTag("aws/other/secrets"),
		},
	}
	s.cloudMetadataStore = &fakeCloudMetadataStore{CallMocker: jujutesting.NewCallMocker(logger)}

	defaultClusterMetadata := &jujucaas.ClusterMetadata{
		Cloud: "gce", Regions: set.NewStrings("us-east1"),
		OperatorStorageClass: &jujucaas.StorageProvisioner{Name: "operator-sc"},
	}
	s.fakeK8sClusterMetadataChecker = &fakeK8sClusterMetadataChecker{CallMocker: jujutesting.NewCallMocker(logger)}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(defaultClusterMetadata, nil)
	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(nil)

	s.initialCloudMap = map[string]cloud.Cloud{
		"mrcloud1": {Name: "mrcloud1", Type: "kubernetes"},
		"mrcloud2": {Name: "mrcloud2", Type: "kubernetes"},
	}

	s.cloudMetadataStore.Call("PersonalCloudMetadata").Returns(s.initialCloudMap, nil)
	s.cloudMetadataStore.Call("PublicCloudMetadata", []string(nil)).Returns(s.initialCloudMap, false, nil)
	s.cloudMetadataStore.Call("WritePersonalCloudMetadata", s.initialCloudMap).Returns(nil)
}

func (s *addCAASSuite) writeTempKubeConfig(c *gc.C) {
	fullpath := filepath.Join(s.dir, "empty-config")
	err := ioutil.WriteFile(fullpath, []byte(""), 0644)
	c.Assert(err, jc.ErrorIsNil)
	os.Setenv("KUBECONFIG", fullpath)
}

func NewMockClientStore() *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "foouser",
	}
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	store.Models["foo"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/bar",
		Models:       map[string]jujuclient.ModelDetails{"admin/bar": {}},
	}
	return store
}

func (s *addCAASSuite) makeCommand(c *gc.C, cloudTypeExists, emptyClientConfig, shouldFakeNewK8sClientConfig bool) cmd.Command {
	return caas.NewAddCAASCommandForTest(
		s.cloudMetadataStore,
		NewMockClientStore(),
		func() (caas.AddCloudAPI, error) {
			return s.fakeCloudAPI, nil
		},
		func(cloud jujucloud.Cloud, credential jujucloud.Credential) (jujucaas.ClusterMetadataChecker, error) {
			return s.fakeK8sClusterMetadataChecker, nil
		},
		caas.FakeCluster(kubeConfigStr),
		func(caasType string) (clientconfig.ClientConfigFunc, error) {
			if !cloudTypeExists {
				return nil, errors.Errorf("unsupported cloud type '%s'", caasType)
			}
			if !shouldFakeNewK8sClientConfig {
				return clientconfig.NewClientConfigReader(caasType)
			}
			s.writeTempKubeConfig(c)
			if emptyClientConfig {
				return fakeEmptyNewK8sClientConfig, nil
			} else {
				return fakeNewK8sClientConfig, nil
			}
		},
		func() (map[string]*jujucmdcloud.CloudDetails, error) {
			return map[string]*jujucmdcloud.CloudDetails{
				"google": {
					Source:           "public",
					CloudType:        "gce",
					CloudDescription: "Google Cloud Platform",
					AuthTypes:        []string{"jsonfile", "oauth2"},
					Regions: yaml.MapSlice{
						{Key: "us-east1", Value: map[string]string{"Name": "us-east1", "Endpoint": "https://www.googleapis.com"}},
					},
					RegionsMap: map[string]jujucmdcloud.RegionDetails{
						"us-east1": {Name: "us-east1", Endpoint: "https://www.googleapis.com"},
					},
				},
				"aws": {
					Source:           "public",
					CloudType:        "ec2",
					CloudDescription: "amazon Cloud Platform",
					AuthTypes:        []string{"jsonfile", "oauth2"},
					Regions: yaml.MapSlice{
						{Key: "ap-southeast-2", Value: map[string]string{"Name": "ap-southeast-2", "Endpoint": "https://ec2.ap-northeast-2.amazonaws.com"}},
					},
					RegionsMap: map[string]jujucmdcloud.RegionDetails{
						"ap-southeast-2": {Name: "ap-southeast-2", Endpoint: "https://ec2.ap-northeast-2.amazonaws.com"},
					},
				},
				"maas1": {
					CloudType:        "maas",
					CloudDescription: "maas Cloud",
				},
			}, nil
		},
	)
}

func (s *addCAASSuite) runCommand(c *gc.C, stdin io.Reader, com cmd.Command, args ...string) (*cmd.Context, error) {
	ctx := cmdtesting.Context(c)
	c.Logf("run cmd with args: %v", args)
	if err := cmdtesting.InitCommand(com, args); err != nil {
		cmd.WriteError(ctx.Stderr, err)
		return ctx, err
	}
	if stdin != nil {
		ctx.Stdin = stdin
	}
	return ctx, com.Run(ctx)
}

func (s *addCAASSuite) TestAddExtraArg(c *gc.C) {
	cmd := s.makeCommand(c, true, true, true)
	_, err := s.runCommand(c, nil, cmd, "k8sname", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *addCAASSuite) TestEmptyKubeConfigFileWithoutStdin(c *gc.C) {
	cmd := s.makeCommand(c, true, true, true)
	_, err := s.runCommand(c, nil, cmd, "k8sname")
	c.Assert(err, gc.ErrorMatches, `No k8s cluster definitions found in config`)
}

func (s *addCAASSuite) TestAddNameClash(c *gc.C) {
	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "mrcloud1")
	c.Assert(err, gc.ErrorMatches, `"mrcloud1" is the name of a public cloud`)
}

func (s *addCAASSuite) TestMissingName(c *gc.C) {
	cmd := s.makeCommand(c, true, true, true)
	_, err := s.runCommand(c, nil, cmd)
	c.Assert(err, gc.ErrorMatches, `missing k8s name.`)
}

func (s *addCAASSuite) TestMissingArgs(c *gc.C) {
	cmd := s.makeCommand(c, true, true, true)
	_, err := s.runCommand(c, nil, cmd)
	c.Assert(err, gc.ErrorMatches, `missing k8s name.`)
}

func (s *addCAASSuite) TestNonExistClusterName(c *gc.C) {
	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "--cluster-name", "non existing cluster name")
	c.Assert(err, gc.ErrorMatches, `context for cluster name "non existing cluster name" not found`)
}

type initTestsCase struct {
	args           []string
	expectedErrStr string
}

func (s *addCAASSuite) TestInit(c *gc.C) {
	for _, ts := range []initTestsCase{
		{
			args:           []string{"--context-name", "a", "--cluster-name", "b"},
			expectedErrStr: "only specify one of cluster-name or context-name, not both",
		},
		{
			args:           []string{"--gke", "--context-name", "a"},
			expectedErrStr: "do not specify context name when adding a GKE cluster",
		},
		{
			args:           []string{"--project", "a"},
			expectedErrStr: "do not specify project unless adding a GKE cluster",
		},
		{
			args:           []string{"--credential", "a"},
			expectedErrStr: "do not specify credential unless adding a GKE cluster",
		},
	} {
		args := append([]string{"myk8s"}, ts.args...)
		cmd := s.makeCommand(c, true, false, true)
		_, err := s.runCommand(c, nil, cmd, args...)
		c.Check(err, gc.ErrorMatches, ts.expectedErrStr)
	}
}

type regionTestCase struct {
	title          string
	cloud, region  string
	expectedErrStr string
}

func (s *addCAASSuite) TestCloudAndRegionFlag(c *gc.C) {
	for i, ts := range []regionTestCase{
		{
			title:          "missing cloud --region=/region",
			region:         "/region",
			expectedErrStr: `parsing cloud region: parsing region option: host cloud region "/region" not valid`,
		},
		{
			title:          "missing region --region=cloud/",
			region:         "cloud/",
			expectedErrStr: `validating cloud region "cloud": cloud region "cloud" not valid`,
		},
		{
			title:          "missing cloud --region=region",
			region:         "region",
			expectedErrStr: `validating cloud region "region": cloud region "region" not valid`,
		},
		{
			title:          "not a known juju cloud region: --region=cloud/region",
			region:         "cloud/region",
			expectedErrStr: `validating cloud region "cloud/region": cloud region "cloud/region" not valid`,
		},
		{
			title:          "region is not required --region=maas/non-existing-region",
			region:         "maas/non-existing-region",
			expectedErrStr: `validating cloud region "maas/non-existing-region": cloud "maas" does not have a region, but "non-existing-region" provided`,
		},
		{
			title:          "region is not required --cloud=maas --region=non-existing-region",
			cloud:          "maas",
			region:         "non-existing-region",
			expectedErrStr: `validating cloud region "maas/non-existing-region": cloud "maas" does not have a region, but "non-existing-region" provided`,
		},
		{
			title:          "missing region --cloud=ec2",
			cloud:          "ec2",
			expectedErrStr: `validating cloud region "ec2": cloud region "ec2" not valid`,
		},
		{
			title:          "cloud option contains region --cloud=gce/us-east1",
			cloud:          "gce/us-east1",
			expectedErrStr: `parsing cloud region: --cloud incorrectly specifies a cloud/region instead of just a cloud`,
		},
		{
			title:          "two different clouds --cloud=gce --region=gce/us-east1",
			cloud:          "ec2",
			region:         "gce/us-east1",
			expectedErrStr: `parsing cloud region: two different clouds specified: "gce", "ec2" not valid`,
		},
		{
			title:  "all good --region=gce/us-east1",
			region: "gce/us-east1",
		},
		{
			title:  "all good --region=us-east1 --cloud=gce",
			region: "us-east1",
			cloud:  "gce",
		},
		{
			title: "all good --cloud=maas",
			cloud: "maas",
		},
		{
			title:  "all good --region=maas",
			region: "maas",
		},
	} {
		c.Logf("%v: %s", i, ts.title)
		delete(s.initialCloudMap, "myk8s")
		cmd := s.makeCommand(c, true, false, true)
		args := []string{
			"myk8s", "-c", "foo", "--cluster-name", "mrcloud2",
		}
		if ts.region != "" {
			args = append(args, "--region", ts.region)
		}
		if ts.cloud != "" {
			args = append(args, "--cloud", ts.cloud)
		}
		_, err := s.runCommand(c, nil, cmd, args...)
		if ts.expectedErrStr == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, ts.expectedErrStr)
		}
	}
}

func (s *addCAASSuite) TestGatherClusterRegionMetaRegionNoMatchesThenIgnored(c *gc.C) {
	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "-c", "foo", "--cluster-name", "mrcloud2")
	c.Assert(err, jc.ErrorIsNil)
	s.cloudMetadataStore.CheckCall(c, 2, "WritePersonalCloudMetadata",
		map[string]cloud.Cloud{
			"mrcloud1": {
				Name:             "mrcloud1",
				Type:             "kubernetes",
				Description:      "",
				HostCloudRegion:  "",
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
				Description:      "",
				HostCloudRegion:  "",
				AuthTypes:        cloud.AuthTypes(nil),
				Endpoint:         "",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
			},
			"myk8s": {
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				HostCloudRegion:  "gce/us-east1",
				AuthTypes:        cloud.AuthTypes{""},
				Endpoint:         "fakeendpoint2",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region{{Name: "us-east1", Endpoint: "fakeendpoint2"}},
				Config:           map[string]interface{}{"operator-storage": "operator-sc", "workload-storage": ""},
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"fakecadata2"},
			},
		},
	)
}

func (s *addCAASSuite) assertAddCloudResult(
	c *gc.C,
	cloudRegion, workloadStorage, operatorStorage string,
	localOnly bool,
) {
	_, region, err := jujucloud.SplitHostCloudRegion(cloudRegion)
	c.Assert(err, jc.ErrorIsNil)
	s.fakeK8sClusterMetadataChecker.CheckCall(c, 0, "GetClusterMetadata")
	expectedCloudToAdd := cloud.Cloud{
		Name:             "myk8s",
		HostCloudRegion:  cloudRegion,
		Type:             "kubernetes",
		Description:      "",
		AuthTypes:        cloud.AuthTypes{""},
		Endpoint:         "fakeendpoint2",
		IdentityEndpoint: "",
		StorageEndpoint:  "",
		Config:           map[string]interface{}{"operator-storage": operatorStorage, "workload-storage": workloadStorage},
		RegionConfig:     cloud.RegionConfig(nil),
		CACertificates:   []string{"fakecadata2"},
	}
	if region != "" {
		expectedCloudToAdd.Regions = []cloud.Region{{Name: region, Endpoint: "fakeendpoint2"}}
	}

	if localOnly {
		s.fakeCloudAPI.CheckNoCalls(c)
	} else {
		s.fakeCloudAPI.CheckCall(c, 0, "AddCloud", expectedCloudToAdd)
	}
	s.cloudMetadataStore.CheckCall(c, 2, "WritePersonalCloudMetadata",
		map[string]cloud.Cloud{
			"mrcloud1": {
				Name:             "mrcloud1",
				Type:             "kubernetes",
				Description:      "",
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
				Description:      "",
				AuthTypes:        cloud.AuthTypes(nil),
				Endpoint:         "",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
			},
			"myk8s": expectedCloudToAdd,
		},
	)
}

func (s *addCAASSuite) TestGatherClusterRegionMetaRegionMatchesAndPassThrough(c *gc.C) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"

	cmd := s.makeCommand(c, true, false, true)
	ctx, err := s.runCommand(c, nil, cmd, "myk8s", "-c", "foo", "--cluster-name", "mrcloud2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.Trim(cmdtesting.Stdout(ctx), "\n"), gc.Equals, `k8s substrate "mrcloud2" added as cloud "myk8s".`)
	s.assertAddCloudResult(c, cloudRegion, "", "operator-sc", false)
}

func (s *addCAASSuite) TestGatherClusterMetadataError(c *gc.C) {
	var result *jujucaas.ClusterMetadata
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(result, errors.New("oops"))

	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "-c", "foo", "--cluster-name", "mrcloud2")
	expectedErr := `
	Juju needs to query the k8s cluster to ensure that the recommended
	storage defaults are available and to detect the cluster's cloud/region.
	This was not possible in this case so run add-k8s again, using
	--storage=<name> to specify the storage class to use and
	--cloud=<cloud> --region=<cloud>/<someregion> to specify the cloud/region.
: oops`[1:]
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *addCAASSuite) TestGatherClusterMetadataNoRegions(c *gc.C) {
	var result jujucaas.ClusterMetadata
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(&result, nil)

	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "--cluster-name", "mrcloud2")
	expectedErr := `
	Juju needs to query the k8s cluster to ensure that the recommended
	storage defaults are available and to detect the cluster's cloud/region.
	This was not possible in this case so run add-k8s again, using
	--storage=<name> to specify the storage class to use and
	--cloud=<cloud> --region=<cloud>/<someregion> to specify the cloud/region.
`[1:]
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *addCAASSuite) TestGatherClusterMetadataUnknownError(c *gc.C) {
	result := &jujucaas.ClusterMetadata{
		Cloud:   "foo",
		Regions: set.NewStrings("region"),
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(result, nil)
	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(errors.NotFoundf("foo"))

	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "--cluster-name", "mrcloud2")
	expectedErr := `
	Juju needs to query the k8s cluster to ensure that the recommended
	storage defaults are available and to detect the cluster's cloud/region.
	This was not possible in this case because the cloud "foo" is not known to Juju.
	Run add-k8s again, using --storage=<name> to specify the storage class to use.
`[1:]
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *addCAASSuite) TestGatherClusterMetadataNoRecommendedStorageError(c *gc.C) {
	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(
		&jujucaas.NonPreferredStorageError{PreferredStorage: jujucaas.PreferredStorage{Name: "disk"}})

	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "--cluster-name", "mrcloud2")
	expectedErr := `
	No recommended storage configuration is defined on this cluster.
	Run add-k8s again with --storage=<name> and Juju will use the
	specified storage class or create a storage-class using the recommended
	"disk" provisioner.
`[1:]
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *addCAASSuite) TestUnknownClusterExistingStorageClass(c *gc.C) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"

	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(errors.NotFoundf("cluster"))
	storageProvisioner := &jujucaas.StorageProvisioner{
		Name:        "mystorage",
		Provisioner: "kubernetes.io/gce-pd",
	}
	s.fakeK8sClusterMetadataChecker.Call("EnsureStorageProvisioner", jujucaas.StorageProvisioner{
		Name: "mystorage",
	}).Returns(storageProvisioner, nil)

	cmd := s.makeCommand(c, true, false, true)
	ctx, err := s.runCommand(c, nil, cmd, "myk8s", "-c", "foo", "--cluster-name", "mrcloud2", "--storage", "mystorage")
	c.Assert(err, jc.ErrorIsNil)
	result := strings.Trim(cmdtesting.Stdout(ctx), "\n")
	result = strings.Replace(result, "\n", " ", -1)
	c.Assert(result, gc.Equals, `k8s substrate "mrcloud2" added as cloud "myk8s" with storage provisioned by the existing "mystorage" storage class.`)
	s.assertAddCloudResult(c, cloudRegion, "mystorage", "mystorage", false)
}

func (s *addCAASSuite) TestCreateDefaultStorageProvisioner(c *gc.C) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"

	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(
		&jujucaas.NonPreferredStorageError{PreferredStorage: jujucaas.PreferredStorage{
			Name:        "gce disk",
			Provisioner: "kubernetes.io/gce-pd"}})
	storageProvisioner := &jujucaas.StorageProvisioner{
		Name:        "mystorage",
		Provisioner: "kubernetes.io/gce-pd",
	}
	s.fakeK8sClusterMetadataChecker.Call("EnsureStorageProvisioner", jujucaas.StorageProvisioner{
		Name:        "mystorage",
		Provisioner: "kubernetes.io/gce-pd",
	}).Returns(storageProvisioner, nil)

	cmd := s.makeCommand(c, true, false, true)
	ctx, err := s.runCommand(c, nil, cmd, "myk8s", "-c", "foo", "--cluster-name", "mrcloud2", "--storage", "mystorage")
	c.Assert(err, jc.ErrorIsNil)
	result := strings.Trim(cmdtesting.Stdout(ctx), "\n")
	result = strings.Replace(result, "\n", " ", -1)
	c.Assert(result, gc.Equals, `k8s substrate "mrcloud2" added as cloud "myk8s" with gce disk default storage provisioned by the existing "mystorage" storage class.`)
	s.assertAddCloudResult(c, cloudRegion, "mystorage", "mystorage", false)
}

func (s *addCAASSuite) TestCreateCustomStorageProvisioner(c *gc.C) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"

	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(
		&jujucaas.NonPreferredStorageError{PreferredStorage: jujucaas.PreferredStorage{Name: "gce disk"}})
	storageProvisioner := &jujucaas.StorageProvisioner{
		Name:        "mystorage",
		Provisioner: "my disk provisioner",
	}
	s.fakeK8sClusterMetadataChecker.Call("EnsureStorageProvisioner", jujucaas.StorageProvisioner{
		Name: "mystorage",
	}).Returns(storageProvisioner, nil)

	cmd := s.makeCommand(c, true, false, true)
	ctx, err := s.runCommand(c, nil, cmd, "myk8s", "-c", "foo", "--cluster-name", "mrcloud2", "--storage", "mystorage")
	c.Assert(err, jc.ErrorIsNil)
	result := strings.Trim(cmdtesting.Stdout(ctx), "\n")
	result = strings.Replace(result, "\n", " ", -1)
	c.Assert(result, gc.Equals, `k8s substrate "mrcloud2" added as cloud "myk8s" with storage provisioned by the existing "mystorage" storage class.`)
	s.assertAddCloudResult(c, cloudRegion, "mystorage", "mystorage", false)
}

func (s *addCAASSuite) TestFoundStorageProvisionerViaAnnationForMAASWIthoutStorageOptionProvided(c *gc.C) {
	storageProvisioner := &jujucaas.StorageProvisioner{
		Name:        "mystorage",
		Provisioner: "my disk provisioner",
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(&jujucaas.ClusterMetadata{
		Cloud:                 "maas",
		OperatorStorageClass:  storageProvisioner,
		NominatedStorageClass: storageProvisioner,
	}, nil)
	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(errors.NotFoundf("no sc config for this cloud type"))
	s.fakeK8sClusterMetadataChecker.Call("EnsureStorageProvisioner", jujucaas.StorageProvisioner{
		Name: "mystorage",
	}).Returns(storageProvisioner, nil)

	cmd := s.makeCommand(c, true, false, true)
	ctx, err := s.runCommand(c, nil, cmd, "myk8s", "-c", "foo", "--cluster-name", "mrcloud2", "--cloud", "maas")
	c.Assert(err, jc.ErrorIsNil)
	result := strings.Trim(cmdtesting.Stdout(ctx), "\n")
	result = strings.Replace(result, "\n", " ", -1)
	c.Assert(result, gc.Equals, `k8s substrate "mrcloud2" added as cloud "myk8s" with storage provisioned by the existing "mystorage" storage class.`)
	s.assertAddCloudResult(c, "maas", "mystorage", "mystorage", false)
}

func (s *addCAASSuite) TestLocalOnly(c *gc.C) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"

	cmd := s.makeCommand(c, true, false, true)
	ctx, err := s.runCommand(c, nil, cmd, "myk8s", "--cluster-name", "mrcloud2", "--local")
	c.Assert(err, jc.ErrorIsNil)
	expected := `k8s substrate "mrcloud2" added as cloud "myk8s".You can now bootstrap to this cloud by running 'juju bootstrap myk8s'.`
	c.Assert(strings.Replace(cmdtesting.Stdout(ctx), "\n", "", -1), gc.Equals, expected)
	s.assertAddCloudResult(c, cloudRegion, "", "operator-sc", true)
}

func mockStdinPipe(content string) (*os.File, error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer pw.Close()
		io.WriteString(pw, content)
	}()
	return pr, nil
}

func (s *addCAASSuite) TestCorrectParseFromStdIn(c *gc.C) {
	cmd := s.makeCommand(c, true, true, false)
	stdIn, err := mockStdinPipe(kubeConfigStr)
	defer stdIn.Close()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.runCommand(c, stdIn, cmd, "myk8s", "-c", "foo")
	c.Assert(err, jc.ErrorIsNil)
	s.assertStoreClouds(c, "gce/us-east1")
}

func (s *addCAASSuite) TestAddGkeCluster(c *gc.C) {
	cmd := s.makeCommand(c, true, true, false)
	_, err := s.runCommand(c, nil, cmd, "-c", "foo", "--gke", "myk8s", "--region", "us-east1")
	c.Assert(err, jc.ErrorIsNil)
	s.assertStoreClouds(c, "gce/us-east1")
}

func (s *addCAASSuite) TestGivenCloudMatch(c *gc.C) {
	err := caas.CheckCloudRegion("gce", "gce/us-east1")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *addCAASSuite) TestGivenCloudMismatch(c *gc.C) {
	err := caas.CheckCloudRegion("maas", "gce")
	c.Assert(err, gc.ErrorMatches, `specified cloud "maas" was different to the detected cloud "gce": re-run the command without specifying the cloud`)

	err = caas.CheckCloudRegion("maas", "gce/us-east1")
	c.Assert(err, gc.ErrorMatches, `specified cloud "maas" was different to the detected cloud "gce": re-run the command without specifying the cloud`)
}

func (s *addCAASSuite) TestGivenRegionMatch(c *gc.C) {
	err := caas.CheckCloudRegion("/us-east1", "gce/us-east1")
	c.Assert(err, jc.ErrorIsNil)

	err = caas.CheckCloudRegion("us-east1", "gce/us-east1")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *addCAASSuite) TestGivenRegionMismatch(c *gc.C) {
	err := caas.CheckCloudRegion("gce/us-east1", "gce/us-east10")
	c.Assert(err, gc.ErrorMatches, `specified region "us-east1" was different to the detected region "us-east10": re-run the command without specifying the region`)
}

func (s *addCAASSuite) assertStoreClouds(c *gc.C, hostCloud string) {
	s.cloudMetadataStore.CheckCall(c, 2, "WritePersonalCloudMetadata",
		map[string]cloud.Cloud{
			"myk8s": {
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes{"userpass"},
				Endpoint:         "https://1.1.1.1:8888",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				HostCloudRegion:  hostCloud,
				Regions: []cloud.Region{
					{Name: "us-east1", Endpoint: "https://1.1.1.1:8888"},
				},
				Config: map[string]interface{}{
					"operator-storage": "operator-sc",
					"workload-storage": "",
				},
				RegionConfig:   cloud.RegionConfig(nil),
				CACertificates: []string{"A"},
			},
			"mrcloud1": {
				Name:             "mrcloud1",
				Type:             "kubernetes",
				Description:      "",
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
				Description:      "",
				AuthTypes:        cloud.AuthTypes(nil),
				Endpoint:         "",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
			},
		},
	)
}

func (s *addCAASSuite) TestCorrectUseCurrentContext(c *gc.C) {
	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "-c", "foo", "myk8s")
	c.Assert(err, jc.ErrorIsNil)
	s.cloudMetadataStore.CheckCall(c, 2, "WritePersonalCloudMetadata",
		map[string]cloud.Cloud{
			"mrcloud1": {
				Name:             "mrcloud1",
				Type:             "kubernetes",
				Description:      "",
				HostCloudRegion:  "",
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
				Description:      "",
				HostCloudRegion:  "",
				AuthTypes:        cloud.AuthTypes(nil),
				Endpoint:         "",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
			},
			"myk8s": {
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				HostCloudRegion:  "gce/us-east1",
				AuthTypes:        cloud.AuthTypes{""},
				Endpoint:         "fakeendpoint1",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region{{Name: "us-east1", Endpoint: "fakeendpoint1"}},
				Config:           map[string]interface{}{"operator-storage": "operator-sc", "workload-storage": ""},
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"fakecadata1"},
			},
		},
	)
}

func (s *addCAASSuite) TestCorrectSelectContext(c *gc.C) {
	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "-c", "foo", "--cluster-name", "mrcloud2")
	c.Assert(err, jc.ErrorIsNil)
	s.cloudMetadataStore.CheckCall(c, 2, "WritePersonalCloudMetadata",
		map[string]cloud.Cloud{
			"mrcloud1": {
				Name:             "mrcloud1",
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes(nil),
				HostCloudRegion:  "",
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
				Description:      "",
				AuthTypes:        cloud.AuthTypes(nil),
				HostCloudRegion:  "",
				Endpoint:         "",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
			},
			"myk8s": {
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				HostCloudRegion:  "gce/us-east1",
				AuthTypes:        cloud.AuthTypes{""},
				Endpoint:         "fakeendpoint2",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region{{Name: "us-east1", Endpoint: "fakeendpoint2"}},
				Config:           map[string]interface{}{"operator-storage": "operator-sc", "workload-storage": ""},
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"fakecadata2"},
			},
		},
	)
}

func (s *addCAASSuite) TestOnlyOneClusterProvider(c *gc.C) {
	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "-c", "foo", "--aks", "--gke")
	c.Assert(err, gc.ErrorMatches, "only one of '--gke' or '--aks' can be supplied")
}
