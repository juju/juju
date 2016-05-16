// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&internalStateSuite{})

// internalStateSuite manages a *State instance for tests in the state
// package (i.e. internal tests) that need it. It is similar to
// state.testing.StateSuite but is duplicated to avoid cyclic imports.
type internalStateSuite struct {
	jujutesting.MgoSuite
	testing.BaseSuite
	state *State
	owner names.UserTag
	model names.ModelTag
}

func getInfo() *mongo.MongoInfo {
	// Copied from NewMongoInfo (due to import loops).
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{jujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
	}
}

func (s *internalStateSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
	s.owner = names.NewLocalUserTag("test-admin")
	st, err := Initialize(s.owner, getInfo(), testing.ModelConfig(c), mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	s.model = st.modelTag
	st.Close()
}

func (s *internalStateSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *internalStateSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)
	st, err := Open(s.model, getInfo(), mongotest.DialOpts(), nil)
	c.Logf("opening state")
	c.Assert(err, jc.ErrorIsNil)
	s.state = st
	s.AddCleanup(func(c *gc.C) {
		s.state.Close()
		c.Logf("state closed")
	})
}

func (s *internalStateSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	c.Logf("repopulating model")
	err := PopulateEmptyModel(s.state, s.owner, getInfo(), testing.ModelConfig(c))
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("leaving internalStateSuite.TearDownTest")
	s.BaseSuite.TearDownTest(c)
}
