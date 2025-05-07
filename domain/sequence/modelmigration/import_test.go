// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"github.com/juju/testing"
	gomock "go.uber.org/mock/gomock"
)

type importSuite struct {
	testing.IsolationSuite

	importService *MockImportService
}

var _ = tc.Suite(&importSuite{})

func (s *importSuite) TestImportSequences(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.importService.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{
		"foo": 1,
		"bar": 2,
	}).Return(nil)

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})
	model.SetSequence("foo", 1)
	model.SetSequence("bar", 2)

	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})

	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() importOperation {
	return importOperation{
		service: s.importService,
	}
}
