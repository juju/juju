// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
)

type slaSuiteV4 struct {
	uniterSuite
}

var _ = gc.Suite(&slaSuiteV4{})

func (s *slaSuiteV4) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
	s.PatchValue(&uniter.NewState, uniter.NewStateV4)
}

func (s *slaSuiteV4) TestSLALevelOldFacadeVersion(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	st := uniter.NewState(apiCaller, names.NewUnitTag("wordpress/0"))
	level, err := st.SLALevel()
	c.Assert(err, jc.ErrorIsNil)

	// testing.APICallerFunc declared the BestFacadeVersion to be 0: that is why we
	// expect "unsupported", because we are talking to an old apiserver.
	c.Assert(level, gc.Equals, "unsupported")
}

type slaSuite struct {
	uniterSuite
}

var _ = gc.Suite(&slaSuite{})

func (s *slaSuite) TestSLALevel(c *gc.C) {
	err := s.State.SetSLA("essential", []byte("creds"))
	c.Assert(err, jc.ErrorIsNil)

	level, err := s.uniter.SLALevel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(level, gc.Equals, "essential")
}
