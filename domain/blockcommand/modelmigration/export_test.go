// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation() *exportOperation {
	return &exportOperation{
		service: s.service,
	}
}

func (s *exportSuite) TestExport(c *gc.C) {
	defer s.setupMocks(c).Finish()

}
