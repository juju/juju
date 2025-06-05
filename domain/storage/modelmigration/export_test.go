// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	domainstorage "github.com/juju/juju/domain/storage"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

func TestExportSuite(t *testing.T) {
	tc.Run(t, &exportSuite{})
}

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

	s.service.EXPECT().ListStoragePoolsWithoutBuiltins(gomock.Any()).
		Times(1).
		Return([]domainstorage.StoragePool{
			{
				Name:     "ebs-fast",
				Provider: "ebs",
				Attrs:    map[string]string{"foo": "bar"},
			},
		}, nil)

	op := s.newExportOperation()
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	pools := dst.StoragePools()
	c.Assert(pools, tc.HasLen, 1)
	sp := pools[0]
	c.Check(sp.Name(), tc.Equals, "ebs-fast")
	c.Check(sp.Provider(), tc.Equals, "ebs")
	c.Assert(sp.Attributes(), tc.DeepEquals, map[string]any{"foo": "bar"})
}
