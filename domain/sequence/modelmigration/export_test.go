// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	jujutesting "github.com/juju/testing"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type exportSuite struct {
	jujutesting.IsolationSuite

	exportService *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) TestExportSequences(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.exportService.EXPECT().GetSequencesForExport(gomock.Any()).Return(map[string]uint64{"seq1": 12, "seq2": 66}, nil)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, gc.IsNil)

	sequences := dst.Sequences()
	c.Assert(sequences, gc.HasLen, 2)
	c.Check(sequences, gc.DeepEquals, map[string]int{"seq1": 12, "seq2": 66})
}

func (s *exportSuite) TestExportSequencesEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	s.exportService.EXPECT().GetSequencesForExport(gomock.Any()).Return(nil, nil)

	op := s.newExportOperation()
	err := op.Execute(context.Background(), dst)
	c.Assert(err, gc.IsNil)

	sequences := dst.Sequences()
	c.Assert(sequences, gc.HasLen, 0)
}

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.exportService = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation() exportOperation {
	return exportOperation{
		service: s.exportService,
	}
}
