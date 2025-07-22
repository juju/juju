// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package imagemetadatamanager -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/imagemetadatamanager ModelConfigService,ModelInfoService,MetadataService

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
	)

	return ctrl
}
