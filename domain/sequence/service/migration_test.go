// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"
	gomock "go.uber.org/mock/gomock"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetSequencesForExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetSequencesForExport(gomock.Any()).Return(map[string]uint64{"foo": 12}, nil)

	seqs, err := s.state.GetSequencesForExport(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(seqs, tc.DeepEquals, map[string]uint64{"foo": 12})
}

func (s *serviceSuite) TestImportSequences(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{"foo": 12}).Return(nil)

	err := s.state.ImportSequences(context.Background(), map[string]uint64{"foo": 12})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveAllSequences(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RemoveAllSequences(gomock.Any()).Return(nil)

	err := s.state.RemoveAllSequences(context.Background())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
