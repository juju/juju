// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/actions"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type actionsSuite struct {
	jujutesting.JujuConnSuite

	actions    *actions.ActionsAPI
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&actionsSuite{})

func (s *actionsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.actions, err = actions.NewActionsAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.IsNil)
}

// TODO(jcw4) implement
func (s *actionsSuite) TestEnqueue(c *gc.C)       {}
func (s *actionsSuite) TestListAll(c *gc.C)       {}
func (s *actionsSuite) TestListPending(c *gc.C)   {}
func (s *actionsSuite) TestListCompleted(c *gc.C) {}
func (s *actionsSuite) TestCancel(c *gc.C)        {}
