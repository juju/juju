// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestGetSequencesForExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetSequencesForExport(gomock.Any()).Return(map[string]uint64{"foo": 12}, nil)

	seqs, err := s.state.GetSequencesForExport(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(seqs, tc.DeepEquals, map[string]uint64{"foo": 12})
}

func (s *serviceSuite) TestImportSequences(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ImportSequences(gomock.Any(), map[string]uint64{"foo": 12}).Return(nil)

	err := s.state.ImportSequences(c.Context(), map[string]uint64{"foo": 12})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveAllSequences(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RemoveAllSequences(gomock.Any()).Return(nil)

	err := s.state.RemoveAllSequences(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
