// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager_test

import (
	stdtesting "testing"

	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/imagemetadatamanager"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/state/cloudimagemetadata"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package imagemetadatamanager_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/imagemetadatamanager ModelConfigService

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type baseImageMetadataSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api   *imagemetadatamanager.API
	state *mockState

	modelConfigService *MockModelConfigService
	ctrl               *gomock.Controller
}

func (s *baseImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test:")
}

func (s *baseImageMetadataSuite) setupMocks(c *gc.C) {
	ctrl := gomock.NewController(c)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.ctrl = ctrl
}

func (s *baseImageMetadataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{Tag: names.NewUserTag("testuser"), Controller: true, AdminTag: names.NewUserTag("testuser")}

	s.state = s.constructState(testConfig(c))

	// Need to create the mock modelConfigService first so we can pass it to the API
	s.setupMocks(c)

	var err error
	s.api, err = imagemetadatamanager.CreateAPI(s.state, s.modelConfigService,
		func() (environs.Environ, error) {
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
	controllerTag  = "controllerTag"
	model          = "model"
)

func (s *baseImageMetadataSuite) constructState(cfg *config.Config) *mockState {
	return &mockState{
		Stub: &jujutesting.Stub{},
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
		controllerTag: func() names.ControllerTag {
			return names.NewControllerTag("deadbeef-2f18-4fd2-967d-db9663db7bea")
		},
	}
}

type mockState struct {
	*jujutesting.Stub

	findMetadata   func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
	saveMetadata   func(m []cloudimagemetadata.Metadata) error
	deleteMetadata func(imageId string) error
	modelConfig    func() (*config.Config, error)
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
	st.Stub.MethodCall(st, modelConfig)
	return st.modelConfig()
}

func (st *mockState) ControllerTag() names.ControllerTag {
	st.Stub.MethodCall(st, controllerTag)
	return st.controllerTag()
}

func (st *mockState) Model() (imagemetadatamanager.Model, error) {
	st.Stub.MethodCall(st, model)
	return &mockModel{}, nil
}

type mockModel struct{}

func (*mockModel) CloudRegion() string {
	return "some-region"
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
	return coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type": "mock",
	})
}
