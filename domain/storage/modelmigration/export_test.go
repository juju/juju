// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/storage"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = tc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
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

func (s *exportSuite) TestExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})
	dst.AddMachine(description.MachineArgs{
		Id: "666",
	})
	c.Assert(dst.StoragePools(), tc.HasLen, 0)

	sc, err := storage.NewConfig("ebs-fast", "ebs", map[string]any{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)
	builtIn, err := storage.NewConfig("loop", "loop", nil)
	c.Assert(err, tc.ErrorIsNil)
	s.service.EXPECT().AllStoragePools(gomock.Any()).
		Times(1).
		Return([]*storage.Config{sc, builtIn}, nil)

	op := s.newExportOperation()
	err = op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	pools := dst.StoragePools()
	c.Assert(pools, tc.HasLen, 1)
	sp := pools[0]
	c.Check(sp.Name(), tc.Equals, "ebs-fast")
	c.Check(sp.Provider(), tc.Equals, "ebs")
	c.Assert(sp.Attributes(), tc.DeepEquals, map[string]any{"foo": "bar"})
}
