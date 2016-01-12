// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	stdtesting "testing"

	"github.com/juju/names"
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

	calls []string
}

func (s *baseImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test:")
}

func (s *baseImageMetadataSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}

	s.calls = []string{}
	s.state = s.constructState(testConfig(c))

	var err error
	s.api, err = imagemetadata.CreateAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseImageMetadataSuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

const (
	findMetadata  = "findMetadata"
	saveMetadata  = "saveMetadata"
	environConfig = "environConfig"
)

func (s *baseImageMetadataSuite) constructState(cfg *config.Config) *mockState {
	return &mockState{
		findMetadata: func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
			s.calls = append(s.calls, findMetadata)
			return nil, nil
		},
		saveMetadata: func(m cloudimagemetadata.Metadata) error {
			s.calls = append(s.calls, saveMetadata)
			return nil
		},
		environConfig: func() (*config.Config, error) {
			s.calls = append(s.calls, environConfig)
			return cfg, nil
		},
	}
}

type mockState struct {
	findMetadata  func(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
	saveMetadata  func(m cloudimagemetadata.Metadata) error
	environConfig func() (*config.Config, error)
}

func (st *mockState) FindMetadata(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
	return st.findMetadata(f)
}

func (st *mockState) SaveMetadata(m cloudimagemetadata.Metadata) error {
	return st.saveMetadata(m)
}

func (st *mockState) EnvironConfig() (*config.Config, error) {
	return st.environConfig()
}

func testConfig(c *gc.C) *config.Config {
	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"type":         "mock",
		"state-server": true,
		"state-id":     "1",
	})
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = environs.Prepare(cfg, envtesting.BootstrapContext(c), configstore.NewMem())
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}
