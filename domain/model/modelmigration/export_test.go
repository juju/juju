// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/model/testing"
)

type exportSuite struct {
	modelExportService *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (e *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	e.modelExportService = NewMockExportService(ctrl)

	return ctrl
}

func (e *exportSuite) TestModelEnvironVersionExport(c *gc.C) {
	defer e.setupMocks(c).Finish()

	newUUID := testing.GenModelUUID(c)
	model := description.NewModel(description.ModelArgs{
		EnvironVersion: 5,
		Config: map[string]interface{}{
			"uuid": newUUID.String(),
		},
	})
	c.Check(model.Tag().Id(), gc.Equals, newUUID.String())
	c.Check(model.EnvironVersion(), gc.Equals, 5)

	e.modelExportService.EXPECT().GetEnvironVersion(gomock.Any()).Return(3, nil)
	exportOp := exportOperation{
		serviceGetter: func(modelUUID coremodel.UUID) ExportService {
			return e.modelExportService
		},
	}
	_ = exportOp.Execute(context.Background(), model)
	c.Check(model.EnvironVersion(), gc.Equals, 3)
}
