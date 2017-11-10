// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	caascfg "github.com/juju/juju/caas/clientconfig"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/caas"
	"github.com/juju/juju/jujuclient"
)

type addCAASSuite struct {
	jujutesting.IsolationSuite
	fakeCloudAPI        *fakeCloudAPI
	store               *fakeCloudMetadataStore
	fileCredentialStore *fakeCredentialStore
	fakeK8SConfigFunc   caascfg.ClientConfigFunc
}

var _ = gc.Suite(&addCAASSuite{})

type fakeAPIConnection struct {
	api.Connection
}

func (*fakeAPIConnection) Close() error {
	return nil
}

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

type fakeCloudAPI struct {
	caas.CloudAPI
	jujutesting.Stub
	authTypes   []cloud.AuthType
	credentials []names.CloudCredentialTag
}

func (api *fakeCloudAPI) AddCloud(cloud.Cloud) error {
	return nil
}

func (api *fakeCloudAPI) AddCredential(tag string, credential cloud.Credential) error {
	return nil
}

func fakeK8SClientConfig() (*caascfg.ClientConfig, error) {
	return &caascfg.ClientConfig{
		Contexts: map[string]caascfg.Context{"somekey": caascfg.Context{
			CloudName:      "mrcloud",
			CredentialName: "credname",
		},
		},
		CurrentContext: "somekey",
		Clouds: map[string]caascfg.CloudConfig{"mrcloud": caascfg.CloudConfig{
			Endpoint: "fakeendpoint",
			Attributes: map[string]interface{}{
				"CAData": "fakecadata",
			},
		},
		},
	}, nil
}

func fakeEmptyK8SClientConfig() (*caascfg.ClientConfig, error) {
	return &caascfg.ClientConfig{}, nil
}

type fakeCredentialStore struct {
	jujutesting.Stub
}

func (fcs fakeCredentialStore) CredentialForCloud(string) (*cloud.CloudCredential, error) {
	return nil, nil
}

func (fcs fakeCredentialStore) AllCredentials() (map[string]cloud.CloudCredential, error) {
	return map[string]cloud.CloudCredential{}, nil
}

func (fcs fakeCredentialStore) UpdateCredential(cloudName string, details cloud.CloudCredential) error {
	return nil
}

func (s *addCAASSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fakeCloudAPI = &fakeCloudAPI{
		authTypes: []cloud.AuthType{
			cloud.EmptyAuthType,
			cloud.AccessKeyAuthType,
		},
		credentials: []names.CloudCredentialTag{
			names.NewCloudCredentialTag("cloud/admin/default"),
			names.NewCloudCredentialTag("aws/other/secrets"),
		},
	}
	var logger loggo.Logger
	s.store = &fakeCloudMetadataStore{CallMocker: jujutesting.NewCallMocker(logger)}

	mrCloud := cloud.Cloud{Name: "mrcloud", Type: "kubernetes"}

	initialCloudMap := map[string]cloud.Cloud{"mrcloud": mrCloud}

	s.store.Call("PersonalCloudMetadata").Returns(initialCloudMap, nil)

	newCloud := cloud.Cloud{
		Name: "newcloud",
		Type: "kubernetes",
	}
	newCloudMap := map[string]cloud.Cloud{"newcloud": newCloud}

	s.store.Call("PublicCloudMetadata", []string(nil)).Returns(initialCloudMap, false, nil)
	newCloudMap["mrcloud"] = mrCloud
	s.store.Call("WritePersonalCloudMetadata", initialCloudMap).Returns(nil)
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

func (s *addCAASSuite) makeCommand(c *gc.C, cloudTypeExists bool, emptyClientConfig bool) cmd.Command {
	addcmd := caas.NewAddCAASCommandForTest(s.store,
		&fakeCredentialStore{},
		NewMockClientStore(),
		&fakeAPIConnection{},
		func(caller base.APICallCloser) caas.CloudAPI {
			return s.fakeCloudAPI
		},
		func(caasType string) (caascfg.ClientConfigFunc, error) {
			if !cloudTypeExists {
				return nil, errors.Errorf("unsupported cloud type '%s'", caasType)
			}
			if emptyClientConfig {
				return fakeEmptyK8SClientConfig, nil
			} else {
				return fakeK8SClientConfig, nil
			}
		},
	)
	return addcmd
}

func (s *addCAASSuite) runCommand(c *gc.C, cmd cmd.Command, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *addCAASSuite) TestAddExtraArg(c *gc.C) {
	cmd := s.makeCommand(c, true, true)
	_, err := s.runCommand(c, cmd, "kubernetes", "caasname", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *addCAASSuite) TestAddKnownTypeNoData(c *gc.C) {
	cmd := s.makeCommand(c, true, true)
	_, err := s.runCommand(c, cmd, "kubernetes", "caasname")
	c.Assert(err, gc.ErrorMatches, `No CAAS cluster definitions found in config`)
}
func (s *addCAASSuite) TestAddUnknownType(c *gc.C) {
	cmd := s.makeCommand(c, false, true)
	_, err := s.runCommand(c, cmd, "ducttape", "caasname")
	c.Assert(err, gc.ErrorMatches, `unsupported cloud type 'ducttape'`)
}

func (s *addCAASSuite) TestAddNameClash(c *gc.C) {
	cmd := s.makeCommand(c, true, false)
	_, err := s.runCommand(c, cmd, "kubernetes", "mrcloud")
	c.Assert(err, gc.ErrorMatches, `"mrcloud" is the name of a public cloud`)
}

func (s *addCAASSuite) TestMissingName(c *gc.C) {
	cmd := s.makeCommand(c, true, true)
	_, err := s.runCommand(c, cmd, "kubernetes")
	c.Assert(err, gc.ErrorMatches, `missing CAAS name.`)
}

func (s *addCAASSuite) TestMissingArgs(c *gc.C) {
	cmd := s.makeCommand(c, true, true)
	_, err := s.runCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, `missing CAAS type and CAAS name.`)
}

func (s *addCAASSuite) TestCorrect(c *gc.C) {
	cmd := s.makeCommand(c, true, false)
	_, err := s.runCommand(c, cmd, "kubernetes", "myk8s")
	c.Assert(err, jc.ErrorIsNil)
	s.store.CheckCall(c, 2, "WritePersonalCloudMetadata",
		map[string]cloud.Cloud{
			"mrcloud": cloud.Cloud{Name: "mrcloud",
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes(nil),
				Endpoint:         "",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil)},

			"myk8s": cloud.Cloud{
				Name:             "myk8s",
				Type:             "kubernetes",
				Description:      "",
				AuthTypes:        cloud.AuthTypes{""},
				Endpoint:         "fakeendpoint",
				IdentityEndpoint: "",
				StorageEndpoint:  "",
				Regions:          []cloud.Region(nil),
				Config:           map[string]interface{}(nil),
				RegionConfig:     cloud.RegionConfig(nil),
				CACertificates:   []string{"fakecadata"},
			}})
}
