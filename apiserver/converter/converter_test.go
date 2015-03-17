// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package converter_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/upgrader"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type upgraderSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	apiMachine *state.Machine
	upgrader   *upgrader.UpgraderAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// Create a machine to work with
	var err error
	// The first machine created is the only one allowed to
	// JobManageEnviron
	s.apiMachine, err = s.State.AddMachine("quantal", state.JobHostUnits,
		state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	s.rawMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// The default auth is as the machine agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawMachine.Tag(),
	}
	s.upgrader, err = upgrader.NewUpgraderAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgraderSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *upgraderSuite) TestWatchAPIVersionNothing(c *gc.C) {
	// Not an error to watch nothing
	results, err := s.upgrader.WatchAPIVersion(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *upgraderSuite) TestWatchAPIVersion(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.rawMachine.Tag().String()}},
	}
	results, err := s.upgrader.WatchAPIVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].NotifyWatcherId, gc.Not(gc.Equals), "")
	c.Check(results.Results[0].Error, gc.IsNil)
	resource := s.resources.Get(results.Results[0].NotifyWatcherId)
	c.Check(resource, gc.NotNil)

	w := resource.(state.NotifyWatcher)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertNoChange()

	err = statetesting.SetAgentVersion(s.State, version.MustParse("3.4.567.8"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
