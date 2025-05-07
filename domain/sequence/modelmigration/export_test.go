// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	gomock "go.uber.org/mock/gomock"
)

type exportSuite struct {
	jujutesting.IsolationSuite

	exportService *MockExportService
}

var _ = tc.Suite(&exportSuite{})

func (s *exportSuite) TestExportSequences(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.exportService.EXPECT().GetSequencesForExport(gomock.Any()).Return(map[string]uint64{"seq1": 12, "seq2": 66}, nil)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
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
	err := op.Execute(context.Background(), dst)
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
