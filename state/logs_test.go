// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strings"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
)

type LogsSuite struct {
	ConnSuite
	logsColl *mgo.Collection
}

var _ = gc.Suite(&LogsSuite{})

func (s *LogsSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	session := s.State.MongoSession()
	s.logsColl = session.DB("logs").C("logs")
}

func (s *LogsSuite) TestIndexesCreated(c *gc.C) {
	// Indexes should be created on the logs collection when state is opened.
	indexes, err := s.logsColl.Indexes()
	c.Assert(err, jc.ErrorIsNil)
	var keys []string
	for _, index := range indexes {
		keys = append(keys, strings.Join(index.Key, "-"))
	}
	c.Assert(keys, jc.SameContents, []string{
		"_id", // default index
		"e-t", // env-uuid and timestamp
		"e-n", // env-uuid and entity
	})
}

func (s *LogsSuite) TestDbLogger(c *gc.C) {
	logger := state.NewDbLogger(s.State, names.NewMachineTag("22"))
	defer logger.Close()
	t0 := time.Now().Truncate(time.Millisecond) // MongoDB only stores timestamps with ms precision.
	logger.Log(t0, "some.where", "foo.go:99", loggo.INFO, "all is well")
	t1 := t0.Add(time.Second)
	logger.Log(t1, "else.where", "bar.go:42", loggo.ERROR, "oh noes")

	var docs []bson.M
	err := s.logsColl.Find(nil).Sort("t").All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 2)

	c.Assert(docs[0]["t"], gc.Equals, t0)
	c.Assert(docs[0]["e"], gc.Equals, s.State.EnvironUUID())
	c.Assert(docs[0]["n"], gc.Equals, "machine-22")
	c.Assert(docs[0]["m"], gc.Equals, "some.where")
	c.Assert(docs[0]["l"], gc.Equals, "foo.go:99")
	c.Assert(docs[0]["v"], gc.Equals, int(loggo.INFO))
	c.Assert(docs[0]["x"], gc.Equals, "all is well")

	c.Assert(docs[1]["t"], gc.Equals, t1)
	c.Assert(docs[1]["e"], gc.Equals, s.State.EnvironUUID())
	c.Assert(docs[1]["n"], gc.Equals, "machine-22")
	c.Assert(docs[1]["m"], gc.Equals, "else.where")
	c.Assert(docs[1]["l"], gc.Equals, "bar.go:42")
	c.Assert(docs[1]["v"], gc.Equals, int(loggo.ERROR))
	c.Assert(docs[1]["x"], gc.Equals, "oh noes")
}
