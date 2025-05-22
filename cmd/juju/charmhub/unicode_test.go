// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/internal/testhelpers"
)

type unicodeSuite struct {
	testhelpers.IsolationSuite
}

func TestUnicodeSuite(t *stdtesting.T) {
	tc.Run(t, &unicodeSuite{})
}

func (s *unicodeSuite) TestCanUnicode(c *tc.C) {
	result := canUnicode("always", nil)
	c.Assert(result, tc.IsTrue)

	result = canUnicode("never", nil)
	c.Assert(result, tc.IsFalse)
}

func (s *unicodeSuite) TestCanUnicodeWithOSEnviron(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockOSEnviron := mocks.NewMockOSEnviron(ctrl)

	result := canUnicode("always", mockOSEnviron)
	c.Assert(result, tc.IsTrue)

	result = canUnicode("never", mockOSEnviron)
	c.Assert(result, tc.IsFalse)

	mockOSEnviron.EXPECT().IsTerminal().Return(true)
	mockOSEnviron.EXPECT().Getenv("LC_MESSAGES").Return("UTF-8")

	result = canUnicode("auto", mockOSEnviron)
	c.Assert(result, tc.IsTrue)
}
