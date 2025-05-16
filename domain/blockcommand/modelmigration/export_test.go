// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/blockcommand"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

func TestExportSuite(t *stdtesting.T) { tc.Run(t, &exportSuite{}) }
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

	s.service.EXPECT().GetBlocks(gomock.Any()).Return([]blockcommand.Block{
		{Type: blockcommand.ChangeBlock, Message: "foo"},
		{Type: blockcommand.RemoveBlock, Message: "bar"},
		{Type: blockcommand.DestroyBlock, Message: "baz"},
	}, nil)

	op := s.newExportOperation()
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(dst.Blocks(), tc.DeepEquals, map[string]string{
		"all-changes":   "foo",
		"remove-object": "bar",
		"destroy-model": "baz",
	})
}

func (s *exportSuite) TestExportEmptyBlocks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.service.EXPECT().GetBlocks(gomock.Any()).Return([]blockcommand.Block{}, nil)

	op := s.newExportOperation()
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(dst.Blocks(), tc.DeepEquals, map[string]string{})
}
