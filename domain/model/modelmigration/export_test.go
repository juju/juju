// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"

	"github.com/juju/description/v10"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/model/testing"
)

type exportSuite struct {
	modelExportService *MockExportService
}

func TestExportSuite(t *stdtesting.T) {
	tc.Run(t, &exportSuite{})
}

// ptr returns a pointer to the value t passed in.
func ptr[T any](t T) *T {
	return &t
}

func (e *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	e.modelExportService = NewMockExportService(ctrl)

	return ctrl
}

func (e *exportSuite) TestModelEnvironVersionExport(c *tc.C) {
	defer e.setupMocks(c).Finish()

	newUUID := testing.GenModelUUID(c)
	model := description.NewModel(description.ModelArgs{
		EnvironVersion: 5,
		Config: map[string]interface{}{
			"uuid": newUUID.String(),
		},
	})
	c.Check(model.UUID(), tc.Equals, newUUID.String())
	c.Check(model.EnvironVersion(), tc.Equals, 5)

	e.modelExportService.EXPECT().GetEnvironVersion(gomock.Any()).Return(3, nil)
	exportOp := exportEnvironVersionOperation{
		exportOperation: exportOperation{
			serviceGetter: func(modelUUID coremodel.UUID) ExportService {
				return e.modelExportService
			},
		},
	}
	_ = exportOp.Execute(c.Context(), model)
	c.Check(model.EnvironVersion(), tc.Equals, 3)
}

func (e *exportSuite) TestModelConstraintsExport(c *tc.C) {
	defer e.setupMocks(c).Finish()

	newUUID := testing.GenModelUUID(c)
	model := description.NewModel(description.ModelArgs{
		EnvironVersion: 5,
		Config: map[string]interface{}{
			"uuid": newUUID.String(),
		},
	})
	c.Check(model.UUID(), tc.Equals, newUUID.String())
	c.Check(model.EnvironVersion(), tc.Equals, 5)

	e.modelExportService.EXPECT().GetModelConstraints(gomock.Any()).Return(
		constraints.Value{
			Arch:             ptr("arm64"),
			AllocatePublicIP: ptr(true),
			Spaces: ptr([]string{
				"space1", "space2",
			}),
		},
		nil,
	)
	exportOp := exportModelConstraintsOperation{
		exportOperation: exportOperation{
			serviceGetter: func(modelUUID coremodel.UUID) ExportService {
				return e.modelExportService
			},
		},
	}
	_ = exportOp.Execute(c.Context(), model)

	// Test values that we know should be set
	c.Check(model.Constraints().AllocatePublicIP(), tc.IsTrue)
	c.Check(model.Constraints().Architecture(), tc.Equals, "arm64")
	c.Check(model.Constraints().Spaces(), tc.DeepEquals, []string{"space1", "space2"})

	// Test values that we know should not be set
	c.Check(model.Constraints().CpuCores(), tc.Equals, uint64(0))
	c.Check(model.Constraints().CpuPower(), tc.Equals, uint64(0))
	c.Check(model.Constraints().ImageID(), tc.Equals, "")
}
