// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/cmd/juju/charmhub/mocks"
)

type unicodeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&unicodeSuite{})

func (s *unicodeSuite) TestCanUnicode(c *gc.C) {
	result := canUnicode("always", nil)
	c.Assert(result, jc.IsTrue)

	result = canUnicode("never", nil)
	c.Assert(result, jc.IsFalse)
}

func (s *unicodeSuite) TestCanUnicodeWithOSEnviron(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockOSEnviron := mocks.NewMockOSEnviron(ctrl)

	result := canUnicode("always", mockOSEnviron)
	c.Assert(result, jc.IsTrue)

	result = canUnicode("never", mockOSEnviron)
	c.Assert(result, jc.IsFalse)

	mockOSEnviron.EXPECT().IsTerminal().Return(true)
	mockOSEnviron.EXPECT().Getenv("LC_MESSAGES").Return("UTF-8")

	result = canUnicode("auto", mockOSEnviron)
	c.Assert(result, jc.IsTrue)
}
