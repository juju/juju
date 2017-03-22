// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type dumpSuite struct {
	ConnSuite
}

var _ = gc.Suite(&dumpSuite{})

func (s *dumpSuite) TestDumpAll(c *gc.C) {
	// Some of the state workers are responsible for creating
	// collections, so make sure they've started before running
	// the dump.
	state.EnsureWorkersStarted(s.State)
	value, err := s.State.DumpAll()
	c.Assert(err, jc.ErrorIsNil)

	models, ok := value["models"].(map[string]interface{})
	c.Assert(ok, jc.IsTrue)
	c.Assert(models["name"], gc.Equals, "testenv")

	initialCollections := set.NewStrings()
	for name := range value {
		initialCollections.Add(name)
	}
	// check that there are some other collections there
	c.Check(initialCollections.Contains("modelusers"), jc.IsTrue)
	c.Check(initialCollections.Contains("leases"), jc.IsTrue)
	c.Check(initialCollections.Contains("statuses"), jc.IsTrue)
}
