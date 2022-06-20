// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang/mock/gomock"
	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
	storagev1 "k8s.io/api/storage/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/caas/kubernetes/provider/proxy"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/caas"
	"github.com/juju/juju/cmd/juju/caas/mocks"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type addCAASSuite struct {
	jujutesting.IsolationSuite
	dir                           string
	publicCloudMap                map[string]cloud.Cloud
	initialCloudMap               map[string]cloud.Cloud
	fakeCloudAPI                  *fakeAddCloudAPI
	fakeK8sClusterMetadataChecker *fakeK8sClusterMetadataChecker
	cloudMetadataStore            *fakeCloudMetadataStore
	credentialStoreAPI            *mocks.MockCredentialStoreAPI
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
- cluster:
    server: https://1.1.1.1:8888
    certificate-authority-data: QQ==
  name: myk8s
contexts:
- context:
    cluster: the-cluster
    user: the-user
  name: the-context
- context:
    cluster: myk8s
    user: test-user
  name: myk8s-ctx
current-context: the-context
preferences: {}
users:
- name: the-user
  user:
    password: thepassword
    username: theuser
- name: test-user
  user:
    token: xfdfsdfsdsd
`

var invalidTLSKubeConfigStr = `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://1.1.1.1:8888
    certificate-authority-data: QQ==
    insecure-skip-tls-verify: true
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

func (f *fakeCloudMetadataStore) ReadCloudData(path string) ([]byte, error) {
	results := f.MethodCall(f, "ReadCloudData", path)
	if results[0] == nil {
		return nil, jujutesting.TypeAssertError(results[1])
	}
	return []byte(results[0].(string)), jujutesting.TypeAssertError(results[1])
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

func (api *fakeAddCloudAPI) AddCloud(kloud cloud.Cloud, force bool) error {
	api.MethodCall(api, "AddCloud", kloud, force)
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
	k8s.ClusterMetadataChecker
	existingSC bool
}

func (api *fakeK8sClusterMetadataChecker) GetClusterMetadata(storageClass string) (result *k8s.ClusterMetadata, err error) {
	results := api.MethodCall(api, "GetClusterMetadata")
	return results[0].(*k8s.ClusterMetadata), jujutesting.TypeAssertError(results[1])
}

func (api *fakeK8sClusterMetadataChecker) CheckDefaultWorkloadStorage(cluster string, storageProvisioner *k8s.StorageProvisioner) error {
	results := api.MethodCall(api, "CheckDefaultWorkloadStorage")
	return jujutesting.TypeAssertError(results[0])
}

func (api *fakeK8sClusterMetadataChecker) EnsureStorageProvisioner(cfg k8s.StorageProvisioner) (*k8s.StorageProvisioner, bool, error) {
	results := api.MethodCall(api, "EnsureStorageProvisioner", cfg)
	return results[0].(*k8s.StorageProvisioner), api.existingSC, jujutesting.TypeAssertError(results[1])
}

func fakeNewK8sClientConfig(_ string, _ io.Reader, contextName, clusterName string, _ clientconfig.K8sCredentialResolver) (*clientconfig.ClientConfig, error) {
	cCfg := &clientconfig.ClientConfig{
		CurrentContext: "key1",
		Credentials: map[string]cloud.Credential{
			"credname1": cloud.NewCredential(
				"certificate",
				map[string]string{
					"ClientCertificateData": `
-----BEGIN CERTIFICATE-----
MIIDBDCCAeygAwIBAgIJAPUHbpCysNxyMA0GCSqGSIb3DQEBCwUAMBcxFTATBgNV`[1:],
					"Token": "xfdfsdfsdsd",
				},
			),
			"credname2": cloud.NewCredential(
				"certificate",
				map[string]string{
					"ClientCertificateData": `
-----BEGIN CERTIFICATE-----
MIIDBDCCAeygAwIBAgIJAPUHbpCysNxyMA0GCSqGSIb3DQEBCwUAMBcxFTATBgNV`[1:],
					"Token": "xfdfsdfsdsd",
				},
			),
		},
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

func fakeEmptyNewK8sClientConfig(string, io.Reader, string, string, clientconfig.K8sCredentialResolver) (*clientconfig.ClientConfig, error) {
	return &clientconfig.ClientConfig{}, nil
}

func (s *addCAASSuite) setupBroker(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.credentialStoreAPI = mocks.NewMockCredentialStoreAPI(ctrl)
	return ctrl
}

func CreateKubeConfigData(conf string) (string, error) {
	file, err := ioutil.TempFile("", "")
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = file.WriteString(conf)
	if err != nil {
		return "", errors.Trace(err)
	}

	return file.Name(), nil
}

func SetKubeConfigData(conf string) error {
	fname, err := CreateKubeConfigData(conf)
	if err != nil {
		return errors.Trace(err)
	}
	return os.Setenv("KUBECONFIG", fname)
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

	defaultClusterMetadata := &k8s.ClusterMetadata{
		Cloud: "gce", Regions: set.NewStrings("us-east1"),
		OperatorStorageClass: &storagev1.StorageClass{
			ObjectMeta: meta.ObjectMeta{Name: "operator-sc"},
		},
	}
	s.fakeK8sClusterMetadataChecker = &fakeK8sClusterMetadataChecker{
		CallMocker: jujutesting.NewCallMocker(logger),
		existingSC: true,
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(defaultClusterMetadata, nil)
	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(nil)

	s.publicCloudMap = map[string]cloud.Cloud{
		"publiccloud": {Name: "publiccloud", Type: "ec2"},
	}
	s.initialCloudMap = map[string]cloud.Cloud{
		"mrcloud1": {Name: "mrcloud1", Type: "kubernetes"},
		"mrcloud2": {Name: "mrcloud2", Type: "kubernetes"},
	}

	s.cloudMetadataStore.Call("PersonalCloudMetadata").Returns(s.initialCloudMap, nil)
	s.cloudMetadataStore.Call("PublicCloudMetadata", []string(nil)).Returns(s.publicCloudMap, false, nil)
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
		APIEndpoints:   []string{"0.1.2.3:1234"},
		ControllerUUID: "uuid",
		Cloud:          "microk8s",
		Proxy: &jujuclient.ProxyConfWrapper{
			Proxier: proxy.NewProxier(proxy.ProxierConfig{APIHost: "10.0.0.1"}),
		},
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
		s.credentialStoreAPI,
		NewMockClientStore(),
		func() (caas.AddCloudAPI, error) {
			return s.fakeCloudAPI, nil
		},
		func(_ jujuclock.Clock) clientconfig.K8sCredentialResolver {
			return func(_ string, c *clientcmdapi.Config, _ string) (*clientcmdapi.Config, error) {
				return c, nil
			}
		},
		func(cloud jujucloud.Cloud, credential jujucloud.Credential) (k8s.ClusterMetadataChecker, error) {
			return s.fakeK8sClusterMetadataChecker, nil
		},
		caas.FakeCluster(kubeConfigStr),
		func(caasType string) (clientconfig.ClientConfigFunc, error) {
			if !cloudTypeExists {
				return nil, errors.Errorf("unsupported cloud type '%s'", caasType)
			}
			if !shouldFakeNewK8sClientConfig {
				return func(
					credentialUID string, reader io.Reader,
					contextName, clusterName string,
					credentialResolver clientconfig.K8sCredentialResolver,
				) (*clientconfig.ClientConfig, error) {
					fakeFunc, err := clientconfig.NewClientConfigReader(caasType)
					c.Assert(err, jc.ErrorIsNil)
					return fakeFunc(credentialUID, reader, contextName, clusterName, nil)
				}, nil
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
				"teststack": {
					Source:           "private",
					CloudType:        "openstack",
					CloudDescription: "openstack for this test",
					AuthTypes:        []string{"jsonfile", "oauth2"},
					Regions: yaml.MapSlice{
						{Key: "aregion", Value: map[string]string{"Name": "aregion", "Endpoint": "endpoint"}},
					},
					RegionsMap: map[string]jujucmdcloud.RegionDetails{
						"aregion": {Name: "aregion", Endpoint: "endpoint"},
					},
					DefaultRegion: "aregion",
				},
				"brokenteststack": {
					Source:           "private",
					CloudType:        "azure",
					CloudDescription: "azure for this test",
					AuthTypes:        []string{"jsonfile", "oauth2"},
					Regions: yaml.MapSlice{
						{Key: "aregion", Value: map[string]string{"Name": "aregion", "Endpoint": "endpoint"}},
					},
					RegionsMap: map[string]jujucmdcloud.RegionDetails{
						"aregion": {Name: "aregion", Endpoint: "endpoint"},
					},
					DefaultRegion: "notknownregion",
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
	command := s.makeCommand(c, true, true, true)
	_, err := s.runCommand(c, nil, command, "k8sname", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *addCAASSuite) TestEmptyKubeConfigFileWithoutStdin(c *gc.C) {
	command := s.makeCommand(c, true, true, true)
	_, err := s.runCommand(c, nil, command, "k8sname", "--client")
	c.Assert(err, gc.ErrorMatches, `kubernetes context "" not found`)
}

func (s *addCAASSuite) TestPublicCloudAddNameClash(c *gc.C) {
	command := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, command, "publiccloud", "--controller", "foo", "--client")
	c.Assert(err, gc.ErrorMatches, `"publiccloud" is the name of a public cloud`)
}

func (s *addCAASSuite) TestLocalCloudExists(c *gc.C) {
	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)
	command := s.makeCommand(c, true, false, true)
	_, err = s.runCommand(c, nil, command, "mrcloud1", "--controller", "foo", "--client")
	c.Assert(err, gc.ErrorMatches, "use `update-k8s mrcloud1 --client` to override known local definition: k8s \"mrcloud1\" already exists")
}

func (s *addCAASSuite) TestMissingName(c *gc.C) {
	command := s.makeCommand(c, true, true, true)
	_, err := s.runCommand(c, nil, command)
	c.Assert(err, gc.ErrorMatches, `missing k8s name.`)
}

func (s *addCAASSuite) TestMissingArgs(c *gc.C) {
	command := s.makeCommand(c, true, true, true)
	_, err := s.runCommand(c, nil, command)
	c.Assert(err, gc.ErrorMatches, `missing k8s name.`)
}

func (s *addCAASSuite) TestNonExistClusterName(c *gc.C) {
	command := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, command, "myk8s", "--cluster-name", "non existing cluster name", "--client")
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
			expectedErrStr: "do not specify context name when adding a AKS/GKE/EKS cluster",
		},
		{
			args:           []string{"--aks", "--context-name", "a"},
			expectedErrStr: "do not specify context name when adding a AKS/GKE/EKS cluster",
		},
		{
			args:           []string{"--eks", "--context-name", "a"},
			expectedErrStr: "do not specify context name when adding a AKS/GKE/EKS cluster",
		},
		{
			args:           []string{"--gke", "--cloud", "a"},
			expectedErrStr: "do not specify --cloud when adding a GKE, EKS or AKS cluster",
		},
		{
			args:           []string{"--aks", "--cloud", "a"},
			expectedErrStr: "do not specify --cloud when adding a GKE, EKS or AKS cluster",
		},
		{
			args:           []string{"--eks", "--cloud", "a"},
			expectedErrStr: "do not specify --cloud when adding a GKE, EKS or AKS cluster",
		},
		{
			args:           []string{"--gke", "--region", "cloud/region"},
			expectedErrStr: "only specify region, not cloud/region, when adding a GKE, EKS or AKS cluster",
		},
		{
			args:           []string{"--aks", "--region", "cloud/region"},
			expectedErrStr: "only specify region, not cloud/region, when adding a GKE, EKS or AKS cluster",
		},
		{
			args:           []string{"--eks", "--region", "cloud/region"},
			expectedErrStr: "only specify region, not cloud/region, when adding a GKE, EKS or AKS cluster",
		},
		{
			args:           []string{"--project", "a"},
			expectedErrStr: "do not specify project unless adding a GKE cluster",
		},
		{
			args:           []string{"--credential", "a"},
			expectedErrStr: "do not specify credential unless adding a GKE cluster",
		},
		{
			args:           []string{"--project", "a", "--aks"},
			expectedErrStr: "do not specify project unless adding a GKE cluster",
		},
		{
			args:           []string{"--credential", "a", "--aks"},
			expectedErrStr: "do not specify credential unless adding a GKE cluster",
		},
		{
			args:           []string{"--project", "a", "--eks"},
			expectedErrStr: "do not specify project unless adding a GKE cluster",
		},
		{
			args:           []string{"--credential", "a", "--eks"},
			expectedErrStr: "do not specify credential unless adding a GKE cluster",
		},
		{
			args:           []string{"--resource-group", "rg1", "--gke"},
			expectedErrStr: "do not specify resource-group unless adding a AKS cluster",
		},
		{
			args:           []string{"--resource-group", "rg1", "--eks"},
			expectedErrStr: "do not specify resource-group unless adding a AKS cluster",
		},
	} {
		args := append([]string{"myk8s"}, ts.args...)
		command := s.makeCommand(c, true, false, true)
		_, err := s.runCommand(c, nil, command, args...)
		c.Check(err, gc.ErrorMatches, ts.expectedErrStr)
	}
}

type regionTestCase struct {
	title          string
	cloud, region  string
	expectedErrStr string
	gke, aks       bool
}

func (s *addCAASSuite) TestCloudAndRegionFlag(c *gc.C) {
	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)
	for i, ts := range []regionTestCase{
		{
			title:          "missing cloud --region=/region",
			region:         "/region",
			expectedErrStr: `parsing cloud region: parsing region option: host cloud region "/region" not valid`,
		}, {
			title:          "missing region --region=cloud/",
			region:         "cloud/",
			expectedErrStr: `validating cloud region "cloud": cloud region "cloud" not valid`,
		}, {
			title:          "missing cloud --region=region",
			region:         "region",
			expectedErrStr: `parsing cloud region: when --region is used, --cloud is required`,
		}, {
			title:          "not a known juju cloud region: --region=cloud/region",
			region:         "cloud/region",
			expectedErrStr: `validating cloud region "cloud/region": cloud region "cloud/region" not valid`,
		}, {
			title:          "region is not required --region=maas/non-existing-region",
			region:         "maas/non-existing-region",
			expectedErrStr: `validating cloud region "maas/non-existing-region": cloud "maas" does not have a region, but "non-existing-region" provided`,
		}, {
			title:          "region is not required --cloud=maas --region=non-existing-region",
			cloud:          "maas",
			region:         "non-existing-region",
			expectedErrStr: `validating cloud region "maas/non-existing-region": cloud "maas" does not have a region, but "non-existing-region" provided`,
		}, {
			title:          "missing region --cloud=ec2 with no cloud default region",
			cloud:          "ec2",
			expectedErrStr: `validating cloud region "ec2": cloud region "ec2" not valid`,
		}, {
			title:          "missing region --cloud=brokenteststack and cloud's default region is not a cloud region",
			cloud:          "brokenteststack",
			expectedErrStr: `validating cloud region "azure": cloud region "azure" not valid`,
		}, {
			title:          "specify cloud with gke",
			cloud:          "aws",
			gke:            true,
			expectedErrStr: `do not specify --cloud when adding a GKE, EKS or AKS cluster`,
		}, {
			title:          "specify cloud with aks",
			cloud:          "aws",
			aks:            true,
			expectedErrStr: `do not specify --cloud when adding a GKE, EKS or AKS cluster`,
		}, {
			title:          "specify cloud/region with gke",
			region:         "gce/us-east",
			gke:            true,
			expectedErrStr: `only specify region, not cloud/region, when adding a GKE, EKS or AKS cluster`,
		}, {
			title: "missing region --cloud=teststack but cloud has default region",
			cloud: "teststack",
		}, {
			title:          "cloud option contains region --cloud=gce/us-east1",
			cloud:          "gce/us-east1",
			expectedErrStr: `parsing cloud region: --cloud incorrectly specifies a cloud/region instead of just a cloud`,
		}, {
			title:          "specify cloud twice --cloud=gce --region=gce/us-east1",
			cloud:          "ec2",
			region:         "gce/us-east1",
			expectedErrStr: "parsing cloud region: when --cloud is used, --region may only specify a region, not a cloud/region",
		}, {
			title:  "all good --region=gce/us-east1",
			region: "gce/us-east1",
		}, {
			title:  "all good --region=us-east1 --cloud=gce",
			region: "us-east1",
			cloud:  "gce",
		}, {
			title: "all good --cloud=maas",
			cloud: "maas",
		},
	} {
		c.Logf("%v: %s", i, ts.title)
		delete(s.initialCloudMap, "myk8s")
		command := s.makeCommand(c, true, false, true)
		args := []string{
			"myk8s", "-c", "foo", "--cluster-name", "the-cluster",
		}
		if ts.region != "" {
			args = append(args, "--region", ts.region)
		}
		if ts.cloud != "" {
			args = append(args, "--cloud", ts.cloud)
		}
		if ts.gke {
			args = append(args, "--gke")
		}
		if ts.aks {
			args = append(args, "--aks")
		}
		_, err := s.runCommand(c, nil, command, args...)
		if ts.expectedErrStr == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, ts.expectedErrStr)
		}
	}
}

func (s *addCAASSuite) TestGatherClusterRegionMetaRegionNoMatchesThenIgnored(c *gc.C) {
	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	command := s.makeCommand(c, true, false, true)
	_, err = s.runCommand(c, nil, command, "myk8s", "-c", "foo", "--cluster-name", "myk8s", "--client")
	c.Assert(err, jc.ErrorIsNil)
	s.cloudMetadataStore.CheckCall(c, 3, "WritePersonalCloudMetadata",
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
				AuthTypes:        cloud.AuthTypes{"certificate"},
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

type testData struct {
	client     bool
	controller bool
}

func (s *addCAASSuite) assertAddCloudResult(
	c *gc.C, testRun func(),
	cloudRegion, workloadStorage, operatorStorage string,
	t testData,
) {

	if t.client {
		s.credentialStoreAPI.EXPECT().UpdateCredential(
			"myk8s", jujucloud.CloudCredential{
				AuthCredentials: map[string]jujucloud.Credential{
					"myk8s": jujucloud.NewNamedCredential(
						"myk8s",
						jujucloud.AuthType("oauth2"),
						map[string]string{
							"Token":   "xfdfsdfsdsd",
							"rbac-id": "9baa5e46",
						},
						false,
					),
				},
			},
		).Times(1).Return(nil)
	}

	testRun()

	_, region, err := jujucloud.SplitHostCloudRegion(cloudRegion)
	c.Assert(err, jc.ErrorIsNil)
	s.fakeK8sClusterMetadataChecker.CheckCall(c, 0, "GetClusterMetadata")
	expectedCloudToAdd := cloud.Cloud{
		Name:             "myk8s",
		HostCloudRegion:  cloudRegion,
		Type:             "kubernetes",
		Description:      "",
		AuthTypes:        cloud.AuthTypes{"certificate", "clientcertificate", "oauth2", "oauth2withcert", "userpass"},
		Endpoint:         "https://1.1.1.1:8888",
		IdentityEndpoint: "",
		StorageEndpoint:  "",
		Config:           map[string]interface{}{"operator-storage": operatorStorage, "workload-storage": workloadStorage},
		RegionConfig:     cloud.RegionConfig(nil),
		CACertificates:   []string{"A"},
	}
	if region != "" {
		expectedCloudToAdd.Regions = []cloud.Region{{Name: region, Endpoint: "https://1.1.1.1:8888"}}
	}
	if !t.controller {
		s.fakeCloudAPI.CheckNoCalls(c)
	} else {
		s.fakeCloudAPI.CheckCall(c, 0, "AddCloud", expectedCloudToAdd, false)
	}
	if t.client {
		s.cloudMetadataStore.CheckCall(c, 3, "WritePersonalCloudMetadata",
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
}

func (s *addCAASSuite) TestGatherClusterRegionMetaRegionMatchesAndPassThrough(c *gc.C) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"

	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddCloudResult(c, func() {
		command := s.makeCommand(c, true, false, true)
		ctx, err := s.runCommand(c, nil, command, "myk8s", "-c", "foo", "--cluster-name", "myk8s", "--client", "-c", "foo")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(strings.Trim(cmdtesting.Stdout(ctx), "\n"), gc.Equals, `k8s substrate "myk8s" added as cloud "myk8s".
You can now bootstrap to this cloud by running 'juju bootstrap myk8s'.`)
	}, cloudRegion, "", "operator-sc", testData{client: true, controller: true})
}

func (s *addCAASSuite) TestGatherClusterMetadataError(c *gc.C) {
	var result *k8s.ClusterMetadata
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(result, errors.New("oops"))

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	command := s.makeCommand(c, true, false, true)
	_, err = s.runCommand(c, nil, command, "myk8s", "-c", "foo", "--cluster-name", "myk8s")
	expectedErr := `
	Juju needs to query the k8s cluster to ensure that the recommended
	storage defaults are available and to detect the cluster's cloud/region.
	This was not possible in this case so run add-k8s again, using
	--storage=<name> to specify the storage class to use and
	--cloud=<cloud> to specify the cloud.
: oops`[1:]
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *addCAASSuite) TestGatherClusterMetadataNoRegions(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	result := &k8s.ClusterMetadata{
		OperatorStorageClass: &storagev1.StorageClass{
			ObjectMeta: meta.ObjectMeta{Name: "mystorage"},
		},
		WorkloadStorageClass: &storagev1.StorageClass{
			ObjectMeta: meta.ObjectMeta{Name: "mystorage"},
		},
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(result, nil)

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddCloudResult(c, func() {
		command := s.makeCommand(c, true, false, true)
		ctx, err := s.runCommand(c, nil, command, "myk8s", "-c", "foo", "--cluster-name", "myk8s", "--client", "-c", "foo")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(strings.Trim(cmdtesting.Stdout(ctx), "\n"), gc.Equals, `k8s substrate "myk8s" added as cloud "myk8s".
You can now bootstrap to this cloud by running 'juju bootstrap myk8s'.`)
	}, "other", "mystorage", "mystorage", testData{client: true, controller: true})
}

func (s *addCAASSuite) TestGatherClusterMetadataUnknownError(c *gc.C) {
	result := &k8s.ClusterMetadata{
		Cloud:   "foo",
		Regions: set.NewStrings("region"),
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(result, nil)
	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(errors.NotFoundf("foo"))

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	command := s.makeCommand(c, true, false, true)
	_, err = s.runCommand(c, nil, command, "myk8s", "--cluster-name", "myk8s", "-c", "foo")
	c.Assert(err, gc.ErrorMatches, `	No recommended storage configuration is defined on this cluster.
	Run add-k8s again with --storage=<name> and Juju will use the
	specified storage class.
`)
}

func (s *addCAASSuite) TestGatherClusterMetadataNoStorageError(c *gc.C) {
	result := &k8s.ClusterMetadata{}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(result, nil)
	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(errors.NotFoundf("foo"))

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	command := s.makeCommand(c, true, false, true)
	_, err = s.runCommand(c, nil, command, "myk8s", "--cluster-name", "myk8s", "-c", "foo")
	c.Assert(err, gc.ErrorMatches, `	No recommended storage configuration is defined on this cluster.
	Run add-k8s again with --storage=<name> and Juju will use the
	specified storage class.
`)
}

func (s *addCAASSuite) TestGatherClusterMetadataUserStorage(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	result := &k8s.ClusterMetadata{
		OperatorStorageClass: &storagev1.StorageClass{
			ObjectMeta: meta.ObjectMeta{Name: "mystorage"},
		},
		WorkloadStorageClass: &storagev1.StorageClass{
			ObjectMeta: meta.ObjectMeta{Name: "mystorage"},
		},
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(result, nil)

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddCloudResult(c, func() {
		command := s.makeCommand(c, true, false, true)
		ctx, err := s.runCommand(c, nil, command, "myk8s", "-c", "foo", "--cluster-name", "myk8s", "--client", "-c", "foo", "--storage", "mystorage")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(strings.Trim(cmdtesting.Stdout(ctx), "\n"), gc.Equals, `k8s substrate "myk8s" added as cloud "myk8s" with storage provisioned
by the existing "mystorage" storage class.
You can now bootstrap to this cloud by running 'juju bootstrap myk8s'.`)
	}, "other", "mystorage", "mystorage", testData{client: true, controller: true})
}

func (s *addCAASSuite) TestGatherClusterMetadataNoRecommendedStorageError(c *gc.C) {
	result := k8s.ClusterMetadata{}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(&result, nil)

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	command := s.makeCommand(c, true, false, true)
	_, err = s.runCommand(c, nil, command, "myk8s", "--cluster-name", "myk8s", "-c", "foo")
	expectedErr := `
	No recommended storage configuration is defined on this cluster.
	Run add-k8s again with --storage=<name> and Juju will use the
	specified storage class.
`[1:]
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *addCAASSuite) TestUnknownClusterExistingStorageClass(c *gc.C) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"

	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	defaultClusterMetadata := &k8s.ClusterMetadata{
		Cloud: cloudRegion,
		OperatorStorageClass: &storagev1.StorageClass{
			ObjectMeta: meta.ObjectMeta{Name: "mystorage"},
		},
		WorkloadStorageClass: &storagev1.StorageClass{
			ObjectMeta: meta.ObjectMeta{Name: "mystorage"},
		},
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(defaultClusterMetadata, nil)

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddCloudResult(c, func() {
		command := s.makeCommand(c, true, false, true)
		ctx, err := s.runCommand(c, nil, command, "myk8s", "-c", "foo", "--cluster-name", "myk8s", "--storage", "mystorage", "--client")
		c.Assert(err, jc.ErrorIsNil)
		result := strings.Trim(cmdtesting.Stdout(ctx), "\n")
		result = strings.Replace(result, "\n", " ", -1)
		c.Assert(result, gc.Equals, `k8s substrate "myk8s" added as cloud "myk8s" with storage provisioned by the existing "mystorage" storage class. You can now bootstrap to this cloud by running 'juju bootstrap myk8s'.`)
	}, cloudRegion, "mystorage", "mystorage", testData{client: true, controller: true})

}

func (s *addCAASSuite) TestSkipStorage(c *gc.C) {
	result := &k8s.ClusterMetadata{}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(result, nil)
	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(errors.NotFoundf("foo"))

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	command := s.makeCommand(c, true, false, true)
	ctx, err := s.runCommand(c, nil, command, "myk8s", "--cluster-name", "myk8s", "-c", "foo", "--skip-storage")
	c.Assert(err, jc.ErrorIsNil)
	out := strings.Trim(cmdtesting.Stdout(ctx), "\n")
	out = strings.Replace(out, "\n", " ", -1)
	c.Assert(out, gc.Equals, `k8s substrate "myk8s" added as cloud "myk8s" with no configured storage provisioning capability on controller foo.`)
}

func (s *addCAASSuite) assertCreateDefaultStorageProvisioner(c *gc.C, expectedMsg string, t testData, additionalArgs ...string) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"

	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	s.fakeK8sClusterMetadataChecker.Call("CheckDefaultWorkloadStorage").Returns(
		&environs.PreferredStorageNotFound{"error"})
	storageProvisioner := &k8s.StorageProvisioner{
		Name:        "mystorage",
		Provisioner: "kubernetes.io/gce-pd",
	}
	s.fakeK8sClusterMetadataChecker.Call("EnsureStorageProvisioner", k8s.StorageProvisioner{
		Name:        "mystorage",
		Provisioner: "kubernetes.io/gce-pd",
	}).Returns(storageProvisioner, nil)

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddCloudResult(c, func() {
		command := s.makeCommand(c, true, false, true)
		args := []string{"myk8s", "--cluster-name", "myk8s", "--storage", "mystorage"}
		if t.controller {
			args = append(args, "-c", "foo")
		}
		if t.client {
			args = append(args, "--client")
		}
		if len(additionalArgs) > 0 {
			args = append(args, additionalArgs...)
		}
		ctx, err := s.runCommand(c, nil, command, args...)
		c.Assert(err, jc.ErrorIsNil)
		result := strings.Trim(cmdtesting.Stdout(ctx), "\n")
		result = strings.Replace(result, "\n", " ", -1)
		c.Assert(result, gc.Equals, expectedMsg)
	}, cloudRegion, "mystorage", "mystorage", t)
}

func (s *addCAASSuite) TestFoundStorageProvisionerViaAnnationForMAASWIthoutStorageOptionProvided(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	sc := &storagev1.StorageClass{
		ObjectMeta:  meta.ObjectMeta{Name: "mystorage"},
		Provisioner: "my disk provisioner",
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(&k8s.ClusterMetadata{
		Cloud:                "maas",
		OperatorStorageClass: sc,
		WorkloadStorageClass: sc,
	}, nil)

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddCloudResult(c, func() {
		command := s.makeCommand(c, true, false, true)
		ctx, err := s.runCommand(c, nil, command, "myk8s", "-c", "foo", "--cluster-name", "myk8s", "--cloud", "maas", "--client")
		c.Assert(err, jc.ErrorIsNil)
		result := strings.Trim(cmdtesting.Stdout(ctx), "\n")
		result = strings.Replace(result, "\n", " ", -1)
		c.Assert(result, gc.Equals, `k8s substrate "myk8s" added as cloud "myk8s". You can now bootstrap to this cloud by running 'juju bootstrap myk8s'.`)
	}, "maas", "mystorage", "mystorage", testData{client: true, controller: true})
}

func (s *addCAASSuite) TestLocalOnly(c *gc.C) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"

	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)

	defaultClusterMetadata := &k8s.ClusterMetadata{
		Cloud: cloudRegion,
		OperatorStorageClass: &storagev1.StorageClass{
			ObjectMeta: meta.ObjectMeta{Name: "operator-sc"},
		},
	}
	s.fakeK8sClusterMetadataChecker.Call("GetClusterMetadata").Returns(defaultClusterMetadata, nil)

	s.assertAddCloudResult(c, func() {
		command := s.makeCommand(c, true, false, true)
		ctx, err := s.runCommand(c, nil, command, "myk8s", "--cluster-name", "myk8s", "--client")
		c.Assert(err, jc.ErrorIsNil)
		expected := `k8s substrate "myk8s" added as cloud "myk8s".You can now bootstrap to this cloud by running 'juju bootstrap myk8s'.`
		c.Assert(strings.Replace(cmdtesting.Stdout(ctx), "\n", "", -1), gc.Equals, expected)
	}, cloudRegion, "", "operator-sc", testData{client: true})
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
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	s.credentialStoreAPI.EXPECT().UpdateCredential(
		"myk8s", jujucloud.CloudCredential{
			AuthCredentials: map[string]jujucloud.Credential{
				"myk8s": jujucloud.NewNamedCredential(
					"myk8s",
					jujucloud.AuthType("userpass"),
					map[string]string{
						"password": "thepassword",
						"rbac-id":  "9baa5e46",
						"username": "theuser",
					},
					false,
				),
			},
		},
	).Times(1).Return(nil)

	command := s.makeCommand(c, true, true, false)
	stdIn, err := mockStdinPipe(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdIn, gc.NotNil)
	defer stdIn.Close()
	_, err = s.runCommand(c, stdIn, command, "myk8s", "-c", "foo", "--client")
	c.Assert(err, jc.ErrorIsNil)
	s.assertStoreClouds(c, "gce/us-east1")
}

func (s *addCAASSuite) TestCorrectPromptOrderFromStdIn(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	command := s.makeCommand(c, true, true, false)
	stdIn, err := mockStdinPipe(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdIn, gc.NotNil)
	defer stdIn.Close()
	ctx, err := s.runCommand(c, stdIn, command, "myk8s")
	c.Assert(errors.Cause(err), gc.ErrorMatches, regexp.QuoteMeta(`
The command is piped and Juju cannot prompt to clarify whether the --client or a --controller is to be used.
Please clarify by re-running the command with the desired option(s).`[1:]))
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "This operation can be applied to both a copy on this client and to the one on a controller.\n")
}

func (s *addCAASSuite) TestSkipTLSVerifyWithCertInvalid(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	command := s.makeCommand(c, true, true, false)
	stdIn, err := mockStdinPipe(invalidTLSKubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stdIn, gc.NotNil)
	defer stdIn.Close()
	_, err = s.runCommand(c, stdIn, command, "myk8s", "-c", "foo", "--client")
	c.Assert(err, gc.ErrorMatches, "cloud with both skip-TLS-verify=true and CA certificates not valid")
}

func (s *addCAASSuite) TestAddGkeCluster(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	s.credentialStoreAPI.EXPECT().UpdateCredential(
		"myk8s", jujucloud.CloudCredential{
			AuthCredentials: map[string]jujucloud.Credential{
				"myk8s": jujucloud.NewNamedCredential(
					"myk8s",
					jujucloud.AuthType("userpass"),
					map[string]string{
						"password": "thepassword",
						"rbac-id":  "9baa5e46",
						"username": "theuser",
					},
					false,
				),
			},
		},
	).Times(1).Return(nil)

	command := s.makeCommand(c, true, true, false)
	_, err := s.runCommand(c, nil, command, "-c", "foo", "--gke", "myk8s", "--region", "us-east1", "--client")
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
	s.cloudMetadataStore.CheckCall(c, 3, "WritePersonalCloudMetadata",
		map[string]cloud.Cloud{
			"myk8s": {
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes{"certificate", "clientcertificate", "oauth2", "oauth2withcert", "userpass"},
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
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()
	s.credentialStoreAPI.EXPECT().UpdateCredential("the-cluster", gomock.Any()).Times(1).Return(nil)
	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)
	command := s.makeCommand(c, true, false, true)
	_, err = s.runCommand(c, nil, command, "-c", "foo", "the-cluster", "--client")
	c.Assert(err, jc.ErrorIsNil)
	s.cloudMetadataStore.CheckCall(c, 3, "WritePersonalCloudMetadata",
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
			"the-cluster": {
				Name:             "the-cluster",
				Type:             "kubernetes",
				Description:      "",
				HostCloudRegion:  "gce/us-east1",
				AuthTypes:        cloud.AuthTypes{"certificate", "clientcertificate", "oauth2", "oauth2withcert", "userpass"},
				Endpoint:         "https://1.1.1.1:8888",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region{{Name: "us-east1", Endpoint: "https://1.1.1.1:8888"}},
				Config:           map[string]interface{}{"operator-storage": "operator-sc", "workload-storage": ""},
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"A"},
			},
		},
	)
}

func (s *addCAASSuite) TestCorrectSelectContext(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()
	s.credentialStoreAPI.EXPECT().UpdateCredential("myk8s", gomock.Any()).Times(1).Return(nil)
	err := SetKubeConfigData(kubeConfigStr)
	c.Assert(err, jc.ErrorIsNil)
	command := s.makeCommand(c, true, false, true)
	_, err = s.runCommand(c, nil, command, "myk8s", "-c", "foo", "--cluster-name", "the-cluster", "--client")
	c.Assert(err, jc.ErrorIsNil)
	s.cloudMetadataStore.CheckCall(c, 3, "WritePersonalCloudMetadata",
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
				AuthTypes:        cloud.AuthTypes{"certificate", "clientcertificate", "oauth2", "oauth2withcert", "userpass"},
				Endpoint:         "https://1.1.1.1:8888",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region{{Name: "us-east1", Endpoint: "https://1.1.1.1:8888"}},
				Config:           map[string]interface{}{"operator-storage": "operator-sc", "workload-storage": ""},
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"A"},
			},
		},
	)
}

func (s *addCAASSuite) TestOnlyOneClusterProvider(c *gc.C) {
	command := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, command, "myk8s", "-c", "foo", "--aks", "--gke")
	c.Assert(err, gc.ErrorMatches, "only one of '--gke', '--eks' or '--aks' can be supplied")
}
