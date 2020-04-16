// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
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

func (s *MongodFinderSuite) TestFindSystemMongo36(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
		exp.Exists("/usr/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/bin/mongod", "--version").Return(mongodb36Version, nil),
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/usr/bin/mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         3,
		Minor:         6,
		Point:         3,
		StorageEngine: mongo.WiredTiger,
	})
}

func (s *MongodFinderSuite) TestFindSystemMongo34(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
		exp.Exists("/usr/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/bin/mongod", "--version").Return(mongodb34Version, nil),
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/usr/bin/mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         3,
		Minor:         4,
		Point:         14,
		StorageEngine: mongo.WiredTiger,
	})
}

func (s *MongodFinderSuite) TestFindJujuMongodb(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
		exp.Exists("/usr/bin/mongod").Return(false),
		exp.Exists("/usr/lib/juju/mongo3.2/bin/mongod").Return(false),
		exp.Exists("/usr/lib/juju/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/lib/juju/bin/mongod", "--version").Return(mongodb24Version, nil),
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/usr/lib/juju/bin/mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         2,
		Minor:         4,
		Point:         9,
		StorageEngine: mongo.MMAPV1,
	})
}

func (s *MongodFinderSuite) TestFindJujuMongodbIgnoringSystemMongodb(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	// We will have *both* a /usr/bin/mongod @2.4.9 and /usr/lib/juju/bin/mongod @2.4.9.
	// However, we don't use the system mongod. It might not have --ssl, an it probably also has the Javascript engine
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
		exp.Exists("/usr/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/bin/mongod", "--version").Return(mongodb24Version, nil),
		exp.Exists("/usr/lib/juju/mongo3.2/bin/mongod").Return(false),
		exp.Exists("/usr/lib/juju/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/lib/juju/bin/mongod", "--version").Return(mongodb24Version, nil),
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/usr/lib/juju/bin/mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         2,
		Minor:         4,
		Point:         9,
		StorageEngine: mongo.MMAPV1,
	})
}

func (s *MongodFinderSuite) TestFindJujuMongodb32(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
		exp.Exists("/usr/bin/mongod").Return(false),
		exp.Exists("/usr/lib/juju/mongo3.2/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/lib/juju/mongo3.2/bin/mongod", "--version").Return(mongodb32Version, nil),
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/usr/lib/juju/mongo3.2/bin/mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         3,
		Minor:         2,
		Point:         15,
		StorageEngine: mongo.WiredTiger,
	})
}

func (s *MongodFinderSuite) TestFindJujuMongodb32IgnoringFailedVersion(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
		exp.Exists("/usr/bin/mongod").Return(false),
		exp.Exists("/usr/lib/juju/mongo3.2/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/lib/juju/mongo3.2/bin/mongod", "--version").Return("bad version string", nil),
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/usr/lib/juju/mongo3.2/bin/mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         3,
		Minor:         2,
		StorageEngine: mongo.WiredTiger,
	})
}

func (s *MongodFinderSuite) TestFindJujuMongodb32IgnoringSystemMongo(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
		exp.Exists("/usr/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/bin/mongod", "--version").Return(mongodb26Version, nil),
		exp.Exists("/usr/lib/juju/mongo3.2/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/lib/juju/mongo3.2/bin/mongod", "--version").Return(mongodb32Version, nil),
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/usr/lib/juju/mongo3.2/bin/mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         3,
		Minor:         2,
		Point:         15,
		StorageEngine: mongo.WiredTiger,
	})
}

func (s *MongodFinderSuite) TestStatButNoExecSystemMongo(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(false),
		exp.Exists("/usr/bin/mongod").Return(true),
		exp.GetCommandOutput("/usr/bin/mongod", "--version").Return(
			"bad result", errors.Errorf("unknown error"), // would be an exec.ExitError
		),
		exp.Exists("/usr/lib/juju/mongo3.2/bin/mongod").Return(false),
		exp.Exists("/usr/lib/juju/bin/mongod").Return(false),
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Check(path, gc.Equals, "")
	c.Check(version, gc.Equals, mongo.Version{})
}

func (s *MongodFinderSuite) TestFindJujuMongodbFromSnap(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Exists("/snap/bin/juju-db.mongod").Return(true),
		exp.GetCommandOutput("/snap/bin/juju-db.mongod", "--version").Return(mongodb409Version, nil),
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/snap/bin/juju-db.mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         4,
		Minor:         0,
		Point:         9,
		StorageEngine: mongo.WiredTiger,
	})
}

func (s *MongodFinderSuite) TestParseMongoVersion(c *gc.C) {
	assertVersion := func(major, minor, point int, patch string, content string) {
		v, err := mongo.ParseMongoVersion(content)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(v.Major, gc.Equals, major)
		c.Check(v.Minor, gc.Equals, minor)
		c.Check(v.Point, gc.Equals, point)
		c.Check(v.Patch, gc.Equals, patch)
	}
	assertVersion(2, 4, 9, "", mongodb24Version)
	assertVersion(2, 6, 10, "", mongodb26Version)
	assertVersion(3, 2, 15, "", mongodb32Version)
	assertVersion(3, 4, 14, "", mongodb34Version)
	assertVersion(3, 6, 3, "", mongodb36Version)
	assertVersion(3, 4, 0, "rc3", "db version v3.4.0-rc3\n")
	assertVersion(2, 4, 6, "", "db version v2.4.6\n")
	assertVersion(2, 4, 0, "alpha1", "db version v2.4.0.alpha1")
}

func (s *MongodFinderSuite) TestParseBadVersion(c *gc.C) {
	assertError := func(errMatch string, content string) {
		v, err := mongo.ParseMongoVersion(content)
		c.Check(err, gc.ErrorMatches, errMatch)
		c.Check(v, gc.Equals, mongo.Version{})
	}
	assertError("'mongod --version' reported:\nbad string\n", "bad string\n")
}

// mongodb24Version is the output of 'mongodb --version' on trusty using juju-mongodb
// the version issued by /usr/bin/mongod is virtually identical, only the timestamp is different
const mongodb24Version = `db version v2.4.9
Thu Apr 12 11:11:39.353 git version: nogitversion
`

// mongodb26Version is the output of 'mongodb --version' using the system version on Xenial (not juju-mongodb32)
const mongodb26Version = `db version v2.6.10
2018-04-12T15:27:28.064+0400 git version: nogitversion
2018-04-12T15:27:28.064+0400 OpenSSL version: OpenSSL 1.0.2g  1 Mar 2016
`

// mongodb32Version is the output of 'mongodb --version' on xenial using juju-mongodb32
const mongodb32Version = `db version v3.2.15
git version: e11e3c1b9c9ce3f7b4a79493e16f5e4504e01140
OpenSSL version: OpenSSL 1.0.2g  1 Mar 2016
allocator: tcmalloc
modules: none
build environment:
    distarch: x86_64
    target_arch: x86_64
`

// mongodb34Version is the output of 'mongodb --version' on bionic before 3.6 was added
const mongodb34Version = `db version v3.4.14
git version: fd954412dfc10e4d1e3e2dd4fac040f8b476b268
OpenSSL version: OpenSSL 1.1.0g  2 Nov 2017
allocator: tcmalloc
modules: none
build environment:
    distarch: x86_64
    target_arch: x86_64
`

// mongodb36Version is the ouptut of 'mongodb --version' as taken from Robie's ppa
const mongodb36Version = `db version v3.6.3
git version: 9586e557d54ef70f9ca4b43c26892cd55257e1a5
OpenSSL version: OpenSSL 1.1.0g  2 Nov 2017
allocator: tcmalloc
modules: none
build environment:
    distarch: x86_64
    target_arch: x86_64
`

// mongodb409Version is the output of 'mongodb --version' from the 4.0/stable snap.
const mongodb409Version = `db version v4.0.9
git version: fc525e2d9b0e4bceff5c2201457e564362909765
OpenSSL version: OpenSSL 1.1.0g  2 Nov 2017
allocator: tcmalloc
modules: none
build environment:
    distarch: x86_64
    target_arch: x86_64
`

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
	if runtime.GOOS == "windows" {
		c.Skip("not running 'echo' on windows")
	}
	tools := mongo.OSSearchTools{}
	out, err := tools.GetCommandOutput("/bin/echo", "argument")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(out, gc.Equals, "argument\n")
}

func (s *OSSearchToolsSuite) TestGetCommandOutputExitNonzero(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("not running 'bash' on windows")
	}
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
	if runtime.GOOS == "windows" {
		c.Skip("not running 'bash' on windows")
	}
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
