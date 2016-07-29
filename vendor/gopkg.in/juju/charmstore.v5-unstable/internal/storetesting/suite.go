// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storetesting // import "gopkg.in/juju/charmstore.v5-unstable/internal/storetesting"

import (
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type IsolatedMgoESSuite struct {
	jujutesting.IsolatedMgoSuite
	ElasticSearchSuite
}

func (s *IsolatedMgoESSuite) SetUpSuite(c *gc.C) {
	s.IsolatedMgoSuite.SetUpSuite(c)
	s.ElasticSearchSuite.SetUpSuite(c)
}

func (s *IsolatedMgoESSuite) TearDownSuite(c *gc.C) {
	s.ElasticSearchSuite.TearDownSuite(c)
	s.IsolatedMgoSuite.TearDownSuite(c)
}

func (s *IsolatedMgoESSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.ElasticSearchSuite.SetUpTest(c)
}

func (s *IsolatedMgoESSuite) TearDownTest(c *gc.C) {
	s.ElasticSearchSuite.TearDownTest(c)
	s.IsolatedMgoSuite.TearDownTest(c)
}
