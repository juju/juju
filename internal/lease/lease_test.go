// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type secretaryFinderSuite struct {
	testing.IsolationSuite

	secretary *MockSecretary
}

var _ = gc.Suite(&secretaryFinderSuite{})

func (s *secretaryFinderSuite) TestRegisterNil(c *gc.C) {
	finder := NewSecretaryFinder()
	finder.Register("foo", nil)

	sec, err := finder.SecretaryFor("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sec, gc.IsNil)
}

func (s *secretaryFinderSuite) TestRegister(c *gc.C) {
	defer s.setupMocks(c).Finish()

	finder := NewSecretaryFinder()

	finder.Register("foo", s.secretary)

	sec, err := finder.SecretaryFor("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sec, gc.Equals, s.secretary)
}

func (s *secretaryFinderSuite) TestSecretaryFor(c *gc.C) {
	finder := NewSecretaryFinder()

	sec, err := finder.SecretaryFor("foo")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(sec, gc.IsNil)
}

func (s *secretaryFinderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretary = NewMockSecretary(ctrl)

	return ctrl
}
