// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage_test

import (
	"testing"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/filestorage"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/metadata"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

func Test(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

func newTestState(c *gc.C) *state.State {
	owner := names.NewLocalUserTag("test-admin")
	dbinfo := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: coretesting.CACert,
		},
	}
	cfg := coretesting.EnvironConfig(c)
	dialopts := mongo.DialOpts{
		Timeout: coretesting.LongWait,
	}
	policy := statetesting.MockPolicy{}
	st, err := state.Initialize(owner, dbinfo, cfg, dialopts, &policy)
	c.Assert(err, gc.IsNil)
	return st
}

type baseSuite struct {
	gitjujutesting.MgoSuite
	coretesting.BaseSuite
	State *state.State
}

func (s *baseSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *baseSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.State = newTestState(c)
}

func (s *baseSuite) TearDownTest(c *gc.C) {
	if s.State != nil {
		// If setup fails, we don't have a State yet
		s.State.Close()
	}
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *baseSuite) metadata(c *gc.C) *metadata.Metadata {
	origin := metadata.NewOrigin(
		s.State.EnvironTag().Id(),
		"0",
		"localhost",
	)
	meta := metadata.NewMetadata(*origin, "", nil)
	err := meta.Finish(int64(42), "some hash", "", nil)
	c.Assert(err, gc.IsNil)
	return meta
}

func (s *baseSuite) checkMetadata(
	c *gc.C, doc filestorage.Doc, expected *metadata.Metadata, id string,
) {
	metadata, ok := doc.(*metadata.Metadata)
	c.Assert(ok, jc.IsTrue)

	if id != "" {
		c.Check(metadata.ID(), gc.Equals, id)
	}
	c.Check(metadata.Notes(), gc.Equals, expected.Notes())
	c.Check(metadata.Timestamp().Unix(), gc.DeepEquals, expected.Timestamp().Unix())
	c.Check(metadata.Checksum(), gc.Equals, expected.Checksum())
	c.Check(metadata.ChecksumFormat(), gc.Equals, expected.ChecksumFormat())
	c.Check(metadata.Size(), gc.Equals, expected.Size())
	c.Check(metadata.Origin(), gc.DeepEquals, expected.Origin())
	c.Check(metadata.Stored(), gc.DeepEquals, expected.Stored())
}

/*
func (cs *ConnSuite) SetUpTest(c *gc.C) {
    c.Log("SetUpTest")
    cs.BaseSuite.SetUpTest(c)
    cs.MgoSuite.SetUpTest(c)
    cs.policy = statetesting.MockPolicy{}
    cfg := testing.EnvironConfig(c)
    cs.owner = names.NewLocalUserTag("test-admin")
    cs.State = TestingInitialize(c, cs.owner, cfg, &cs.policy)
    uuid, ok := cfg.UUID()
    c.Assert(ok, jc.IsTrue)
    cs.envTag = names.NewEnvironTag(uuid)
    cs.annotations = cs.MgoSuite.Session.DB("juju").C("annotations")
    cs.charms = cs.MgoSuite.Session.DB("juju").C("charms")
    cs.machines = cs.MgoSuite.Session.DB("juju").C("machines")
    cs.relations = cs.MgoSuite.Session.DB("juju").C("relations")
    cs.services = cs.MgoSuite.Session.DB("juju").C("services")
    cs.units = cs.MgoSuite.Session.DB("juju").C("units")
    cs.stateServers = cs.MgoSuite.Session.DB("juju").C("stateServers")
    cs.factory = factory.NewFactory(cs.State)
    c.Log("SetUpTest done")
}
*/
