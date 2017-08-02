// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	caascfg "github.com/juju/juju/caas/clientconfig"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/caas"
)

type addCAASSuite struct {
	jujutesting.IsolationSuite
	fakeCloudAPI      *fakeCloudAPI
	store             *fakeCloudMetadataStore
	fakeK8SConfigFunc caascfg.ClientConfigFunc
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
	caas.AddCloudAPI
	jujutesting.Stub
	authTypes   []cloud.AuthType
	credentials []names.CloudCredentialTag
}

func fakeK8SClientConfig() (*caascfg.ClientConfig, error) {
	return &caascfg.ClientConfig{}, nil
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
	s.store.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]cloud.Cloud{
		"mrcloud": cloud.Cloud{
			Name: "mrcloud",
			Type: "kubernetes"},
	}, false, nil)
}

func (s *addCAASSuite) makeCommand(c *gc.C, cloudTypeExists bool) *caas.AddCAASCommand {
	return caas.NewAddCAASCommandForTest(s.store, &fakeAPIConnection{},
		func(caller base.APICallCloser) caas.AddCloudAPI {
			return s.fakeCloudAPI
		},
		func(caasType string) (caascfg.ClientConfigFunc, error) {
			if cloudTypeExists {
				return fakeK8SClientConfig, nil
			} else {
				return nil, errors.Errorf("unsupported cloud type '%s'", caasType)
			}
		},
	)
}

func (s *addCAASSuite) runCommand(c *gc.C, cmd *caas.AddCAASCommand, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *addCAASSuite) TestAddExtraArg(c *gc.C) {
	cmd := s.makeCommand(c, true)
	_, err := s.runCommand(c, cmd, "kubernetes", "caasname", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *addCAASSuite) TestAddKnownTypeNoData(c *gc.C) {
	cmd := s.makeCommand(c, true)
	_, err := s.runCommand(c, cmd, "kubernetes", "caasname")
	c.Assert(err, gc.ErrorMatches, `No CAAS cluster definitions found in config`)
}
func (s *addCAASSuite) TestAddUnknownType(c *gc.C) {
	cmd := s.makeCommand(c, false)
	_, err := s.runCommand(c, cmd, "ducttape", "caasname")
	c.Assert(err, gc.ErrorMatches, `unsupported cloud type 'ducttape'`)
}

func (s *addCAASSuite) TestAddNameClash(c *gc.C) {
	cmd := s.makeCommand(c, true)
	_, err := s.runCommand(c, cmd, "kubernetes", "mrcloud")
	c.Assert(err, gc.ErrorMatches, `"mrcloud" is the name of a public cloud`)
}

func (s *addCAASSuite) TestMissingName(c *gc.C) {
	cmd := s.makeCommand(c, true)
	_, err := s.runCommand(c, cmd, "kubernetes")
	c.Assert(err, gc.ErrorMatches, `missing CAAS name.`)
}

func (s *addCAASSuite) TestMissingArgs(c *gc.C) {
	cmd := s.makeCommand(c, true)
	_, err := s.runCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, `missing CAAS type and CAAS name.`)
}
