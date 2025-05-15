// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type importSuite struct {
	testhelpers.IsolationSuite

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

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportSequencesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	op := s.newImportOperation()

	model := description.NewModel(description.ModelArgs{})

	err := op.Execute(c.Context(), model)
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
