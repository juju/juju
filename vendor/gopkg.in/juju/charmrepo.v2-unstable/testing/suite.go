// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing // import "gopkg.in/juju/charmrepo.v2-unstable/testing"

import (
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type IsolatedMgoSuite struct {
	jujutesting.IsolationSuite
	jujutesting.MgoSuite
}

func (s *IsolatedMgoSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *IsolatedMgoSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.IsolationSuite.TearDownSuite(c)
}

func (s *IsolatedMgoSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
}

func (s *IsolatedMgoSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}
