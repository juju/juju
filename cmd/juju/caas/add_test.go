// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/cloud"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/caas"
	jujucmdcloud "github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

type addCAASSuite struct {
	jujutesting.IsolationSuite
	dir                       string
	fakeCloudAPI              *fakeAddCloudAPI
	fakeK8sBrokerRegionLister *fakeK8sBrokerRegionLister
	store                     *fakeCloudMetadataStore
	fileCredentialStore       *fakeCredentialStore
	fakeK8SConfigFunc         *clientconfig.ClientConfigFunc
	currentClusterRegionSet   *set.Strings
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

type fakeK8sBrokerRegionLister struct {
	*jujutesting.CallMocker
	caas.K8sBrokerRegionLister
}

func (api *fakeK8sBrokerRegionLister) ListHostCloudRegions() (set.Strings, error) {
	results := api.MethodCall(api, "ListHostCloudRegions")
	return *results[0].(*set.Strings), jujutesting.TypeAssertError(results[1])
}

func fakeNewK8sClientConfig(io.Reader) (*clientconfig.ClientConfig, error) {
	return &clientconfig.ClientConfig{
		Contexts: map[string]clientconfig.Context{
			"key1": {
				CloudName:      "mrcloud1",
				CredentialName: "credname1",
			},
			"key2": {
				CloudName:      "mrcloud2",
				CredentialName: "credname2",
			},
		},
		CurrentContext: "key1",
		Clouds: map[string]clientconfig.CloudConfig{
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
		},
	}, nil
}

func fakeEmptyNewK8sClientConfig(io.Reader) (*clientconfig.ClientConfig, error) {
	return &clientconfig.ClientConfig{}, nil
}

type fakeCredentialStore struct {
	jujutesting.Stub
}

func (fcs *fakeCredentialStore) CredentialForCloud(string) (*cloud.CloudCredential, error) {
	panic("unexpected call to CredentialForCloud")
}

func (fcs *fakeCredentialStore) AllCredentials() (map[string]cloud.CloudCredential, error) {
	fcs.AddCall("AllCredentials")
	return map[string]cloud.CloudCredential{}, nil
}

func (fcs *fakeCredentialStore) UpdateCredential(cloudName string, details cloud.CloudCredential) error {
	fcs.AddCall("UpdateCredential", cloudName, details)
	return nil
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
	s.currentClusterRegionSet = &set.Strings{}

	s.store = &fakeCloudMetadataStore{CallMocker: jujutesting.NewCallMocker(logger)}

	s.fakeK8sBrokerRegionLister = &fakeK8sBrokerRegionLister{CallMocker: jujutesting.NewCallMocker(logger)}
	s.fakeK8sBrokerRegionLister.Call("ListHostCloudRegions").Returns(s.currentClusterRegionSet, nil)

	initialCloudMap := map[string]cloud.Cloud{
		"mrcloud1": {Name: "mrcloud1", Type: "kubernetes"},
		"mrcloud2": {Name: "mrcloud2", Type: "kubernetes"},
	}

	s.store.Call("PersonalCloudMetadata").Returns(initialCloudMap, nil)

	s.store.Call("PublicCloudMetadata", []string(nil)).Returns(initialCloudMap, false, nil)
	s.store.Call("WritePersonalCloudMetadata", initialCloudMap).Returns(nil)
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

func (s *addCAASSuite) makeCommand(c *gc.C, cloudTypeExists bool, emptyClientConfig bool, shouldFakeNewK8sClientConfig bool) cmd.Command {
	return caas.NewAddCAASCommandForTest(
		s.store,
		&fakeCredentialStore{},
		NewMockClientStore(),
		func() (caas.AddCloudAPI, error) {
			return s.fakeCloudAPI, nil
		},
		func(cloud jujucloud.Cloud, credential jujucloud.Credential) (caas.K8sBrokerRegionLister, error) {
			return s.fakeK8sBrokerRegionLister, nil
		},
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
				"gce": {
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
			}, nil
		},
	)
}

func (s *addCAASSuite) runCommand(c *gc.C, stdin io.Reader, com cmd.Command, args ...string) (*cmd.Context, error) {
	ctx := cmdtesting.Context(c)
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
	c.Assert(err, gc.ErrorMatches, `clusterName \"non existing cluster name\" not found`)
}

type regionTestCase struct {
	title          string
	regionStr      string
	expectedErrStr string
}

func (s *addCAASSuite) TestRegionFlag(c *gc.C) {
	for _, ts := range []regionTestCase{
		{
			title:          "missing cloud",
			regionStr:      "/region",
			expectedErrStr: `validating cloud region "/region": parsing cloud region: cloud region /region not valid`,
		},
		{
			title:          "missing region",
			regionStr:      "cloud/",
			expectedErrStr: `validating cloud region "cloud/": parsing cloud region: cloud region cloud/ not valid`,
		},
		{
			title:          "invalid formnat, it should be <cloudType>/<region>",
			regionStr:      "cloudRegion",
			expectedErrStr: `validating cloud region "cloudRegion": parsing cloud region: cloud region cloudRegion not valid`,
		},
		{
			title:          "not a known juju cloud region",
			regionStr:      "cloud/region",
			expectedErrStr: `validating cloud region "cloud/region": cloud region cloud/region not valid`,
		},
		{
			title:          "all good",
			regionStr:      "gce/us-east1",
			expectedErrStr: "",
		},
	} {
		cmd := s.makeCommand(c, true, false, true)
		_, err := s.runCommand(c, nil, cmd, "myk8s", "--cluster-name", "mrcloud2", "--region", ts.regionStr)
		if ts.expectedErrStr == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, ts.expectedErrStr)
		}
	}
}

func (s *addCAASSuite) TestGatherClusterRegionMetaRegionNoMatchesThenIgnored(c *gc.C) {
	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "--cluster-name", "mrcloud2")
	c.Assert(err, jc.ErrorIsNil)
	s.store.CheckCall(c, 2, "WritePersonalCloudMetadata",
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
			"myk8s": {
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes{""},
				Endpoint:         "fakeendpoint2",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"fakecadata2"},
			},
		},
	)
}

func (s *addCAASSuite) TestGatherClusterRegionMetaRegionMatchesAndPassThrough(c *gc.C) {
	s.fakeCloudAPI.isCloudRegionRequired = true
	cloudRegion := "gce/us-east1"
	*s.currentClusterRegionSet = set.NewStrings(cloudRegion)

	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "--cluster-name", "mrcloud2")
	c.Assert(err, jc.ErrorIsNil)
	// 1st try failed because region was missing.
	s.fakeCloudAPI.CheckCall(c, 0, "AddCloud",
		cloud.Cloud{
			Name:             "myk8s",
			HostCloudRegion:  "", // empty cloud region, but isCloudRegionRequired is true
			Type:             "kubernetes",
			Description:      "",
			AuthTypes:        cloud.AuthTypes{""},
			Endpoint:         "fakeendpoint2",
			IdentityEndpoint: "",
			StorageEndpoint:  "",
			Regions:          []cloud.Region(nil),
			Config:           map[string]interface{}(nil),
			RegionConfig:     cloud.RegionConfig(nil),
			CACertificates:   []string{"fakecadata2"},
		},
	)
	s.fakeK8sBrokerRegionLister.CheckCall(c, 0, "ListHostCloudRegions")
	// 2nd try with region fetched from ListHostCloudRegions.
	s.fakeCloudAPI.CheckCall(c, 1, "AddCloud",
		cloud.Cloud{
			Name:             "myk8s",
			HostCloudRegion:  cloudRegion,
			Type:             "kubernetes",
			Description:      "",
			AuthTypes:        cloud.AuthTypes{""},
			Endpoint:         "fakeendpoint2",
			IdentityEndpoint: "",
			StorageEndpoint:  "",
			Regions:          []cloud.Region(nil),
			Config:           map[string]interface{}(nil),
			RegionConfig:     cloud.RegionConfig(nil),
			CACertificates:   []string{"fakecadata2"},
		},
	)
	s.store.CheckCall(c, 2, "WritePersonalCloudMetadata",
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
			"myk8s": {
				Name:             "myk8s",
				HostCloudRegion:  cloudRegion,
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes{""},
				Endpoint:         "fakeendpoint2",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"fakecadata2"},
			},
		},
	)
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
	_, err = s.runCommand(c, stdIn, cmd, "myk8s")
	c.Assert(err, jc.ErrorIsNil)
	s.store.CheckCall(c, 2, "WritePersonalCloudMetadata",
		map[string]cloud.Cloud{
			"myk8s": {
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes{"userpass"},
				Endpoint:         "https://1.1.1.1:8888",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"A"},
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
	_, err := s.runCommand(c, nil, cmd, "myk8s")
	c.Assert(err, jc.ErrorIsNil)
	s.store.CheckCall(c, 2, "WritePersonalCloudMetadata",
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
			"myk8s": {
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes{""},
				Endpoint:         "fakeendpoint1",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"fakecadata1"},
			},
		},
	)
}

func (s *addCAASSuite) TestCorrectSelectContext(c *gc.C) {
	cmd := s.makeCommand(c, true, false, true)
	_, err := s.runCommand(c, nil, cmd, "myk8s", "--cluster-name", "mrcloud2")
	c.Assert(err, jc.ErrorIsNil)
	s.store.CheckCall(c, 2, "WritePersonalCloudMetadata",
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
			"myk8s": {
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes{""},
				Endpoint:         "fakeendpoint2",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"fakecadata2"},
			},
		},
	)
}
