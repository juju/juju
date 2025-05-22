// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/testhelpers"
)

type MongodFinderSuite struct {
	testhelpers.IsolationSuite

	ctrl   *gomock.Controller
	finder *mongo.MongodFinder
	search *mongo.MockSearchTools
}

func TestMongodFinderSuite(t *stdtesting.T) {
	tc.Run(t, &MongodFinderSuite{})
}

// setUpMock must be called at the start of each test, and then s.ctrl.Finish() called (you can use defer()).
// this cannot be done in SetUpTest() and TearDownTest() because gomock.NewController assumes the TestReporter is valid
// for the entire lifetime of the Controller, and gocheck passes a different C object to SetUpTest vs the Test itself
// vs TearDownTest. And calling c.Fatalf() on the original C object doesn't actually fail the test suite in TearDown.
func (s *MongodFinderSuite) setUpMock(c *tc.C) {
	s.ctrl = gomock.NewController(c)
	s.finder, s.search = mongo.NewMongodFinderWithMockSearch(s.ctrl)
}

func (s *MongodFinderSuite) TestFindJujuMongodb(c *tc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(true),
	)
	path, err := s.finder.InstalledAt()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(path, tc.Equals, "/snap/bin/juju-db.mongod")
}

func (s *MongodFinderSuite) TestFindJujuMongodbNone(c *tc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
	)
	_, err := s.finder.InstalledAt()
	c.Assert(err, tc.ErrorMatches, "juju-db snap not installed, no mongo at /snap/bin/juju-db.mongod")
}

type OSSearchToolsSuite struct {
	testhelpers.IsolationSuite
}

func TestOSSearchToolsSuite(t *stdtesting.T) {
	tc.Run(t, &OSSearchToolsSuite{})
}

func (s *OSSearchToolsSuite) TestExists(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "filename")
	f, err := os.Create(path)
	c.Assert(err, tc.ErrorIsNil)
	f.Close()
	tools := mongo.OSSearchTools{}
	c.Check(tools.Exists(path), tc.IsTrue)
	c.Check(tools.Exists(path+"-not-there"), tc.IsFalse)
}

func (s *OSSearchToolsSuite) TestGetCommandOutputValid(c *tc.C) {
	tools := mongo.OSSearchTools{}
	out, err := tools.GetCommandOutput("/bin/echo", "argument")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(out, tc.Equals, "argument\n")
}

func (s *OSSearchToolsSuite) TestGetCommandOutputExitNonzero(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "failing")
	err := os.WriteFile(path, []byte(`#!/bin/bash --norc
echo "hello $1"
exit 1
`), 0755)
	c.Assert(err, tc.ErrorIsNil)
	tools := mongo.OSSearchTools{}
	out, err := tools.GetCommandOutput(path, "argument")
	c.Assert(err, tc.NotNil)
	c.Check(out, tc.Equals, "hello argument\n")
}

func (s *OSSearchToolsSuite) TestGetCommandOutputNonExecutable(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "failing")
	err := os.WriteFile(path, []byte(`#!/bin/bash --norc
echo "shouldn't happen $1"
`), 0644)
	c.Assert(err, tc.ErrorIsNil)
	tools := mongo.OSSearchTools{}
	out, err := tools.GetCommandOutput(path, "argument")
	c.Assert(err, tc.NotNil)
	c.Check(out, tc.Equals, "")
}
