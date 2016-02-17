// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&StateSuite{})

type StateSuite struct {
	testing.IsolationSuite

	stub    *testing.Stub
	raw     *stubRawState
	persist *stubPersistence
	storage *stubStorage
}

func (s *StateSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.raw = &stubRawState{stub: s.stub}
	s.persist = &stubPersistence{stub: s.stub}
	s.storage = &stubStorage{stub: s.stub}
	s.raw.ReturnPersistence = s.persist
	s.raw.ReturnStorage = s.storage
}

func (s *StateSuite) TestNewStateOkay(c *gc.C) {
	st := NewState(s.raw)

	c.Check(st, gc.NotNil)
	s.stub.CheckCallNames(c, "Persistence", "Storage")
}
