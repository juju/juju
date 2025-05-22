// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type exportSuite struct {
	testhelpers.IsolationSuite

	exportService *MockExportService
}

func TestExportSuite(t *testing.T) {
	tc.Run(t, &exportSuite{})
}

func (s *exportSuite) TestExportSequences(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.exportService.EXPECT().GetSequencesForExport(gomock.Any()).Return(map[string]uint64{"seq1": 12, "seq2": 66}, nil)

	op := s.newExportOperation()
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.IsNil)

	sequences := dst.Sequences()
	c.Assert(sequences, tc.HasLen, 2)
	c.Check(sequences, tc.DeepEquals, map[string]int{"seq1": 12, "seq2": 66})
}

func (s *exportSuite) TestExportSequencesEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.exportService.EXPECT().GetSequencesForExport(gomock.Any()).Return(nil, nil)

	op := s.newExportOperation()
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.IsNil)

	sequences := dst.Sequences()
	c.Assert(sequences, tc.HasLen, 0)
}

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.exportService = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation() exportOperation {
	return exportOperation{
		service: s.exportService,
	}
}
