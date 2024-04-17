// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v5"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/application/service"
)

type importSuite struct {
	importService *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.importService = NewMockImportService(ctrl)
	return ctrl
}

func (i *importSuite) TestApplicationSave(c *gc.C) {
	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Tag: names.NewApplicationTag("prometheus"),
	}
	model.AddApplication(appArgs).AddUnit(description.UnitArgs{
		Tag: names.NewUnitTag("prometheus/0"),
	})

	defer i.setupMocks(c).Finish()

	i.importService.EXPECT().CreateApplication(
		gomock.Any(),
		"prometheus",
		gomock.Any(),
		[]service.AddUnitParams{
			{
				UnitName: ptrString("prometheus/0"),
			},
		},
	).Return(nil)

	importOp := importOperation{
		service: i.importService,
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func ptrString(s string) *string {
	return &s
}
