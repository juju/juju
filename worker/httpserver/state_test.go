// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"github.com/juju/os/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	statetesting "github.com/juju/juju/state/testing"
)

type stateFixture struct {
	testing.IsolationSuite
	statetesting.StateSuite
}

var _ = gc.Suite(&stateFixture{})

func (s *stateFixture) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)

	// Patch the mongo in for bionic as we can't bring in the
	// latest testing repo due to other clock changes.
	host, err := series.HostSeries()
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("os.Series: %v", host)
	if host == "bionic" {
		s.IsolationSuite.PatchEnvironment("JUJU_MONGOD", mongo.MongodSystemPath)
	}

	err = testing.MgoServer.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.IsolationSuite.AddCleanup(func(*gc.C) { testing.MgoServer.Destroy() })

	s.StateSuite.SetUpSuite(c)
}

func (s *stateFixture) TearDownSuite(c *gc.C) {
	s.StateSuite.TearDownSuite(c)
	s.IsolationSuite.TearDownSuite(c)
}

func (s *stateFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.StateSuite.SetUpTest(c)
}

func (s *stateFixture) TearDownTest(c *gc.C) {
	s.StateSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}
