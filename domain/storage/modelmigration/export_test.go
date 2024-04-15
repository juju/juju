// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v6"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/storage"
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

	dst := description.NewModel(description.ModelArgs{})
	dst.AddMachine(description.MachineArgs{
		Id: names.NewMachineTag("666"),
	})
	c.Assert(dst.StoragePools(), gc.HasLen, 0)

	sc, err := storage.NewConfig("ebs-fast", "ebs", map[string]any{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	builtIn, err := storage.NewConfig("loop", "loop", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.service.EXPECT().AllStoragePools(gomock.Any()).
		Times(1).
		Return([]*storage.Config{sc, builtIn}, nil)

	op := s.newExportOperation()
	err = op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)

	pools := dst.StoragePools()
	c.Assert(pools, gc.HasLen, 1)
	sp := pools[0]
	c.Check(sp.Name(), gc.Equals, "ebs-fast")
	c.Check(sp.Provider(), gc.Equals, "ebs")
	c.Assert(sp.Attributes(), jc.DeepEquals, map[string]any{"foo": "bar"})
}
