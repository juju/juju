// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/crossmodel"
)

type removeSuite struct {
	BaseCrossModelSuite
	mockAPI *mockRemoveAPI
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)
	s.mockAPI = &mockRemoveAPI{}
}

func (s *removeSuite) runRemove(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, crossmodel.NewRemoveCommandForTest(s.store, s.mockAPI), args...)
}

func (s *removeSuite) TestRemoveURLError(c *gc.C) {
	_, err := s.runRemove(c, "fred/model.foo/db2")
	c.Assert(err, gc.ErrorMatches, "application offer URL has invalid form.*")
}

func (s *removeSuite) TestRemoveInconsistentControllers(c *gc.C) {
	_, err := s.runRemove(c, "ctrl:fred/model.db2", "ctrl2:fred/model.db2")
	c.Assert(err, gc.ErrorMatches, "all offer URLs must use the same controller")
}

func (s *removeSuite) TestRemoveApiError(c *gc.C) {
	s.mockAPI.msg = "fail"
	_, err := s.runRemove(c, "fred/model.db2")
	c.Assert(err, gc.ErrorMatches, ".*fail.*")
}

func (s *removeSuite) TestRemove(c *gc.C) {
	s.mockAPI.expectedURLs = []string{"fred/model.db2", "mary/model.db2"}
	_, err := s.runRemove(c, "fred/model.db2", "mary/model.db2")
	c.Assert(err, jc.ErrorIsNil)
}

type mockRemoveAPI struct {
	msg          string
	expectedURLs []string
}

func (s mockRemoveAPI) Close() error {
	return nil
}

func (s mockRemoveAPI) DestroyOffers(offerURLs ...string) error {
	if s.msg != "" {
		return errors.New(s.msg)
	}
	if strings.Join(s.expectedURLs, ",") != strings.Join(offerURLs, ",") {
		return errors.New("mismatched URLs")
	}
	return nil
}
