// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/blockcommand"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		service: s.service,
	}
}

func (s *importSuite) TestRegisterImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().SwitchBlockOn(gomock.Any(), blockcommand.ChangeBlock, "foo").Return(nil)
	s.service.EXPECT().SwitchBlockOn(gomock.Any(), blockcommand.RemoveBlock, "bar").Return(nil)
	s.service.EXPECT().SwitchBlockOn(gomock.Any(), blockcommand.DestroyBlock, "baz").Return(nil)

	model := description.NewModel(description.ModelArgs{
		Blocks: map[string]string{
			"all-changes":   "foo",
			"remove-object": "bar",
			"destroy-model": "baz",
		},
	})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportEmptyBlocks(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}
