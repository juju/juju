// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
)

type MongodFinderSuite struct {
	testing.IsolationSuite

	ctrl   *gomock.Controller
	finder *mongo.MongodFinder
	search *mongo.MockSearchTools
}

var _ = gc.Suite(&MongodFinderSuite{})

// setUpMock must be called at the start of each test, and then s.ctrl.Finish() called (you can use defer()).
// this cannot be done in SetUpTest() and TearDownTest() because gomock.NewController assumes the TestReporter is valid
// for the entire lifetime of the Controller, and gocheck passes a different C object to SetUpTest vs the Test itself
// vs TearDownTest. And calling c.Fatalf() on the original C object doesn't actually fail the test suite in TearDown.
func (s *MongodFinderSuite) setUpMock(c *gc.C) {
	s.ctrl = gomock.NewController(c)
	s.finder, s.search = mongo.NewMongodFinderWithMockSearch(s.ctrl)
}

func (s *MongodFinderSuite) TestFindJujuMongodb(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(true),
	)
	path, err := s.finder.InstalledAt()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/snap/bin/juju-db.mongod")
}

func (s *MongodFinderSuite) TestFindJujuMongodbNone(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
	)
	_, err := s.finder.InstalledAt()
	c.Assert(err, gc.ErrorMatches, "juju-db snap not installed, no mongo at /snap/bin/juju-db.mongod")
}

type OSSearchToolsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OSSearchToolsSuite{})

func (s *OSSearchToolsSuite) TestExists(c *gc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "filename")
	f, err := os.Create(path)
	c.Assert(err, jc.ErrorIsNil)
	f.Close()
	tools := mongo.OSSearchTools{}
	c.Check(tools.Exists(path), jc.IsTrue)
	c.Check(tools.Exists(path+"-not-there"), jc.IsFalse)
}

func (s *OSSearchToolsSuite) TestGetCommandOutputValid(c *gc.C) {
	tools := mongo.OSSearchTools{}
	out, err := tools.GetCommandOutput("/bin/echo", "argument")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(out, gc.Equals, "argument\n")
}

func (s *OSSearchToolsSuite) TestGetCommandOutputExitNonzero(c *gc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "failing")
	err := ioutil.WriteFile(path, []byte(`#!/bin/bash --norc
echo "hello $1"
exit 1
`), 0755)
	c.Assert(err, jc.ErrorIsNil)
	tools := mongo.OSSearchTools{}
	out, err := tools.GetCommandOutput(path, "argument")
	c.Assert(err, gc.NotNil)
	c.Check(out, gc.Equals, "hello argument\n")
}

func (s *OSSearchToolsSuite) TestGetCommandOutputNonExecutable(c *gc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "failing")
	err := ioutil.WriteFile(path, []byte(`#!/bin/bash --norc
echo "shouldn't happen $1"
`), 0644)
	c.Assert(err, jc.ErrorIsNil)
	tools := mongo.OSSearchTools{}
	out, err := tools.GetCommandOutput(path, "argument")
	c.Assert(err, gc.NotNil)
	c.Check(out, gc.Equals, "")
}
