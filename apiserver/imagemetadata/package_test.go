// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	stdtesting "testing"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/imagemetadata"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
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

	api   *imagemetadata.API
	state *mockState
}

func (s *baseImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test:")
}

func (s *baseImageMetadataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{Tag: names.NewUserTag("testuser"), EnvironManager: true, AdminTag: names.NewUserTag("testuser")}

	s.state = s.constructState(testConfig(c), &mockModel{"meep"})

	var err error
	s.api, err = imagemetadata.CreateAPI(s.state, func() (environs.Environ, error) {
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
	environConfig  = "environConfig"
)

func (s *baseImageMetadataSuite) constructState(cfg *config.Config, model imagemetadata.Model) *mockState {
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
		environConfig: func() (*config.Config, error) {
			return cfg, nil
		},
		model: func() (imagemetadata.Model, error) {
			return model, nil
		},
		controllerTag: func() names.ControllerTag {
			return names.NewControllerTag("deadbeef-2f18-4fd2-967d-db9663db7bea")
		},
	}
}

type mockState struct {
	*gitjujutesting.Stub

	findMetadata   func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
	saveMetadata   func(m []cloudimagemetadata.Metadata) error
	deleteMetadata func(imageId string) error
	environConfig  func() (*config.Config, error)
	model          func() (imagemetadata.Model, error)
	controllerTag  func() names.ControllerTag
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
	st.Stub.MethodCall(st, environConfig)
	return st.environConfig()
}

func (st *mockState) Model() (imagemetadata.Model, error) {
	st.Stub.MethodCall(st, "Model")
	return st.model()
}

func (st *mockState) ControllerTag() names.ControllerTag {
	st.Stub.MethodCall(st, "ControllerTag")
	return st.controllerTag()
}

type mockModel struct {
	cloudRegion string
}

func (m *mockModel) CloudRegion() string {
	return m.cloudRegion
}

func testConfig(c *gc.C) *config.Config {
	cfg, err := config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type": "mock",
	}))
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}
