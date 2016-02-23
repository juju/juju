// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	stdtesting "testing"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/imagemetadata"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/state/cloudimagemetadata"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

func init() {
	provider := mockEnvironProvider{}
	environs.RegisterProvider("mock", provider)
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
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}

	s.state = s.constructState(testConfig(c))

	var err error
	s.api, err = imagemetadata.CreateAPI(s.state, s.resources, s.authorizer)
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
		environConfig: func() (*config.Config, error) {
			return cfg, nil
		},
	}
}

type mockState struct {
	*gitjujutesting.Stub

	findMetadata   func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
	saveMetadata   func(m []cloudimagemetadata.Metadata) error
	deleteMetadata func(imageId string) error
	environConfig  func() (*config.Config, error)
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

func testConfig(c *gc.C) *config.Config {
	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type":       "mock",
		"controller": true,
		"state-id":   "1",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = environs.Prepare(
		envtesting.BootstrapContext(c), configstore.NewMem(),
		jujuclienttesting.NewMemStore(),
		"dummycontroller", environs.PrepareForBootstrapParams{Config: cfg},
	)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}
