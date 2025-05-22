// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/simplestreams"
	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package imagemetadatamanager -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/imagemetadatamanager ModelConfigService,ModelInfoService,MetadataService

type baseImageMetadataSuite struct {
	coretesting.BaseSuite

	modelConfigService *MockModelConfigService
	modelInfoService   *MockModelInfoService
	metadataService    *MockMetadataService
	api                *API
}

func (s *baseImageMetadataSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test:")
}

func (s *baseImageMetadataSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *baseImageMetadataSuite) setupAPI(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.metadataService = NewMockMetadataService(ctrl)
	s.api = newAPI(
		s.metadataService,
		s.modelConfigService,
		s.modelInfoService,
		func() (environs.Environ, error) {
			return &mockEnviron{}, nil
		},
	)

	return ctrl
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
func (e mockEnviron) Region() (simplestreams.CloudSpec, error) {
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
