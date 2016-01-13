// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apidiscoverspaces "github.com/juju/juju/api/discoverspaces"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/discoverspaces"
)

type workerSuite struct {
	testing.JujuConnSuite

	Worker  worker.Worker
	OpsChan chan dummy.Operation

	APIConnection api.Connection
	API           *apidiscoverspaces.API
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Unbreak dummy provider methods.
	s.AssertConfigParameterUpdated(c, "broken", "")

	s.APIConnection, _ = s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.API = s.APIConnection.DiscoverSpaces()

	//s.State.StartSync()

	s.OpsChan = make(chan dummy.Operation, 10)
	dummy.Listen(s.OpsChan)
}

func (s *workerSuite) startWorker() {
	s.Worker = discoverspaces.NewWorker(s.API)
}

func (s *workerSuite) TearDownTest(c *gc.C) {
	if s.Worker != nil {
		c.Assert(worker.Stop(s.Worker), jc.ErrorIsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *workerSuite) TestConvertSpaceName(c *gc.C) {
	empty := set.Strings{}
	nameTests := []struct {
		name     string
		existing set.Strings
		expected string
	}{
		{"foo", empty, "foo"},
		{"foo1", empty, "foo1"},
		{"Foo Thing", empty, "foo-thing"},
		{"foo^9*//++!!!!", empty, "foo9"},
		{"--Foo", empty, "foo"},
		{"---^^&*()!", empty, "empty"},
		{" ", empty, "empty"},
		{"", empty, "empty"},
		{"foo\u2318", empty, "foo"},
		{"foo", set.NewStrings("foo", "bar", "baz"), "foo-2"},
		{"foo", set.NewStrings("foo", "foo-2"), "foo-3"},
		{"---", set.NewStrings("empty"), "empty-2"},
	}
	for _, test := range nameTests {
		result := discoverspaces.ConvertSpaceName(test.name, test.existing)
		c.Check(result, gc.Equals, test.expected)
	}
}

func (s *workerSuite) TestWorkerIsStringsWorker(c *gc.C) {
	s.startWorker()
	c.Assert(s.Worker, gc.Not(gc.FitsTypeOf), worker.FinishedWorker{})
}

func (s *workerSuite) TestWorkerSupportsSpaceDiscoveryFalse(c *gc.C) {
	s.startWorker()
	spaces, err := s.State.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)

	// No spaces will have been created, worker does nothing.
	c.Assert(spaces, jc.DeepEquals, []*state.Space{})
}

func (s *workerSuite) TestWorkerDiscoversSpaces(c *gc.C) {
	dummy.SetSupportsSpaceDiscovery(true)
	s.startWorker()
	var err error
	var spaces []*state.Space
	for a := common.ShortAttempt.Start(); a.Next(); {
		spaces, err = s.State.AllSpaces()
		if err != nil {
			break
		}
		if len(spaces) == 4 {
			// All spaces have been created.
			break
		}
		if !a.HasNext() {
			c.Fatalf("spaces not imported")
		}
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaces, jc.DeepEquals, nil)
}
