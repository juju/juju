// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/mocks"
)

type stateSuite struct {
	mockContext *mocks.MockContext
}

func (s *stateSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockContext = mocks.NewMockContext(ctrl)
	return ctrl
}

func (s *stateSuite) expectStateSetOne() {
	s.mockContext.EXPECT().SetCharmStateValue(gomock.Any(), "one", "two").Return(nil)
}

func (s *stateSuite) expectStateSetOneEmpty() {
	s.mockContext.EXPECT().SetCharmStateValue(gomock.Any(), "one", "").Return(nil)
}

func (s *stateSuite) expectStateSetTwo() {
	s.expectStateSetOne()
	s.mockContext.EXPECT().SetCharmStateValue(gomock.Any(), "three", "four").Return(nil)
}

func (s *stateSuite) expectStateDeleteOne() {
	s.mockContext.EXPECT().DeleteCharmStateValue(gomock.Any(), "five").Return(nil)
}

func (s *stateSuite) expectStateGetTwo() {
	setupCache := map[string]string{
		"one":   "two",
		"three": "four",
	}
	s.mockContext.EXPECT().GetCharmState(gomock.Any()).Return(setupCache, nil)
}

func (s *stateSuite) expectStateGetValueOne() {
	s.mockContext.EXPECT().GetCharmStateValue(gomock.Any(), "one").Return("two", nil)
}

func (s *stateSuite) expectStateGetValueNotFound() {
	s.mockContext.EXPECT().GetCharmStateValue(gomock.Any(), "five").Return("", errors.NotFoundf("%q", "five"))
}

func (s *stateSuite) expectStateGetValueEmpty() {
	s.mockContext.EXPECT().GetCharmStateValue(gomock.Any(), "five").Return("", nil)
}
