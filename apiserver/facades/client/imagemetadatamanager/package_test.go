// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager_test

import (
	stdtesting "testing"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/imagemetadatamanager"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state/cloudimagemetadata"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type baseImageMetadataSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api   *imagemetadatamanager.API
	state *mockState
}

func (s *baseImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test:")
}

func (s *baseImageMetadataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{Tag: names.NewUserTag("testuser"), Controller: true, AdminTag: names.NewUserTag("testuser")}

	s.state = s.constructState(testConfig(c))

	var err error
	s.api, err = imagemetadatamanager.CreateAPI(s.state, func() (environs.Environ, error) {
		return &mockEnviron{}, nil
	}, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseImageMetadataSuite) assertCalls(c *gc.C, expectedCalls ...string) {
	s.state.Stub.CheckCallNames(c, expectedCalls...)
}

const (
	findMetadata   = "findMetadata"
	saveMetadata   = "saveMetadata"
	deleteMetadata = "deleteMetadata"
	modelConfig    = "modelConfig"
)

func (s *baseImageMetadataSuite) constructState(cfg *config.Config) *mockState {
	return &mockState{
		Stub: &gitjujutesting.Stub{},
		findMetadata: func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
			return nil, nil
		},
		saveMetadata: func(m []cloudimagemetadata.Metadata) error {
			return nil
		},
		deleteMetadata: func(imageId string) error {
			return nil
		},
		modelConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
}

type mockState struct {
	*gitjujutesting.Stub

	findMetadata   func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
	saveMetadata   func(m []cloudimagemetadata.Metadata) error
	deleteMetadata func(imageId string) error
	modelConfig    func() (*config.Config, error)
}

func (st *mockState) FindMetadata(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
	st.Stub.MethodCall(st, findMetadata, f)
	return st.findMetadata(f)
}

func (st *mockState) SaveMetadata(m []cloudimagemetadata.Metadata) error {
	st.Stub.MethodCall(st, saveMetadata, m)
	return st.saveMetadata(m)
}

func (st *mockState) DeleteMetadata(imageId string) error {
	st.Stub.MethodCall(st, deleteMetadata, imageId)
	return st.deleteMetadata(imageId)
}

func (st *mockState) ModelConfig() (*config.Config, error) {
	st.Stub.MethodCall(st, modelConfig)
	return st.modelConfig()
}

func testConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type": "mock",
	}))
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

// mockEnviron is an environment without networking support.
type mockEnviron struct {
	environs.Environ
}

func (e mockEnviron) Config() *config.Config {
	cfg, err := config.New(config.NoDefaults, mockConfig())
	if err != nil {
		panic("invalid configuration for testing")
	}
	return cfg
}

// Region is specified in the HasRegion interface.
func (e *mockEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   "dummy_region",
		Endpoint: "https://anywhere",
	}, nil
}

// mockConfig returns a configuration for the usage of the
// mock provider below.
func mockConfig() coretesting.Attrs {
	return dummy.SampleConfig().Merge(coretesting.Attrs{
		"type": "mock",
	})
}
