// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/domain/resource"
)

type exportSuite struct {
	resourceState *MockExportState
}

func TestExportSuite(t *testing.T) {
	tc.Run(t, &exportSuite{})
}

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.resourceState = NewMockExportState(ctrl)

	return ctrl
}

func (s *exportSuite) newExporter() *Exporter {
	return &Exporter{
		resourceState: s.resourceState,
	}
}

func (s *exportSuite) TestExportResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := resource.ExportedResources{
		Resources: []coreresource.Resource{{RetrievedBy: "a"}},
		UnitResources: []coreresource.UnitResources{{
			Name: "app/0",
			Resources: []coreresource.Resource{{
				RetrievedBy: "b",
			}},
		}},
	}

	s.resourceState.EXPECT().ListAllModelResources(gomock.Any()).Return(expected, nil)

	resources, err := s.newExporter().ExportResources(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resources, tc.DeepEquals, expected)
}

func (s *exportSuite) TestExportResourcesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.resourceState.EXPECT().ListAllModelResources(gomock.Any()).Return(resource.ExportedResources{}, nil)

	resources, err := s.newExporter().ExportResources(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resources, tc.DeepEquals, resource.ExportedResources{})
}
