// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type secretaryFinderSuite struct {
	testhelpers.IsolationSuite

	secretary *MockSecretary
}

func TestSecretaryFinderSuite(t *stdtesting.T) {
	tc.Run(t, &secretaryFinderSuite{})
}

func (s *secretaryFinderSuite) TestRegisterNil(c *tc.C) {
	finder := s.newSecretaryFinder(map[string]lease.Secretary{
		"foo": nil,
	})

	sec, err := finder.SecretaryFor("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sec, tc.IsNil)
}

func (s *secretaryFinderSuite) TestRegister(c *tc.C) {
	defer s.setupMocks(c).Finish()

	finder := s.newSecretaryFinder(map[string]lease.Secretary{
		"foo": s.secretary,
	})

	sec, err := finder.SecretaryFor("foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sec, tc.Equals, s.secretary)
}

func (s *secretaryFinderSuite) TestSecretaryFor(c *tc.C) {
	finder := NewSecretaryFinder(uuid.MustNewUUID().String())

	sec, err := finder.SecretaryFor("foo")
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(sec, tc.IsNil)
}

func (s *secretaryFinderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretary = NewMockSecretary(ctrl)

	return ctrl
}

func (s *secretaryFinderSuite) newSecretaryFinder(secretaries map[string]lease.Secretary) SecretaryFinder {
	return SecretaryFinder{
		secretaries: secretaries,
	}
}
