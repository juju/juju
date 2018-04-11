// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"os"
	"os/exec"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	coretesting "github.com/juju/juju/testing"
	"time"
)

type MongodFinderSuite struct {
	coretesting.BaseSuite

	ctrl   *gomock.Controller
	finder *mongo.MongodFinder
	search *mongo.MockSearchTools
}

var _ = gc.Suite(&MongodFinderSuite{})

type cannedFileInfo struct {
	Name_    string
	Size_    int64
	Mode_    os.FileMode
	ModTime_ time.Time
	IsDir_   bool
}

func (f cannedFileInfo) Name() string       { return f.Name_ }
func (f cannedFileInfo) Size() int64        { return f.Size_ }
func (f cannedFileInfo) Mode() os.FileMode  { return f.Mode_ }
func (f cannedFileInfo) ModTime() time.Time { return f.ModTime_ }
func (f cannedFileInfo) IsDir() bool        { return f.IsDir_ }
func (f cannedFileInfo) Sys() interface{}   { return nil }

// setUpMock must be called at the start of each test, and then s.ctrl.Finish() called (you can use defer()).
// this cannot be done in SetUpTest() and TearDownTest() because gomock.NewController assumes the TestReporter is valid
// for the entire lifetime of the Controller, and gocheck passes a different C object to SetUpTest vs the Test itself
// vs TearDownTest. And calling c.Fatalf() on the original C object doesn't actually fail the test suite in TearDown.
func (s *MongodFinderSuite) setUpMock(c *gc.C) {
	s.ctrl = gomock.NewController(c)
	s.finder, s.search = mongo.NewMongodFinderWithMockSearch(s.ctrl)
}

// expectSystemMongod sets up the expected calls that will happen for /usr/bin/mongod to exist and for it to
// return the expected version string
func (s *MongodFinderSuite) expectSystemMongod(versionInfo string) {
	exp := s.search.EXPECT()
	gomock.InOrder(
		exp.Stat("/usr/bin/mongod").Return(
			cannedFileInfo{
				Name_:    "mongod",
				Size_:    1024 * 1024,
				Mode_:    0755,
				ModTime_: time.Now(),
				IsDir_:   false,
			}, nil),
		exp.RunCommand("/usr/bin/mongod", "--version").Return(versionInfo, nil),
	)
}

func (s *MongodFinderSuite) TestFindSystemMongo36(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	s.expectSystemMongod(mongodb36Version)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/usr/bin/mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         3,
		Minor:         6,
		Patch:         "3",
		StorageEngine: mongo.WiredTiger,
	})
}

func (s *MongodFinderSuite) TestFindSystemMongo34(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	s.expectSystemMongod(mongodb34Version)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(path, gc.Equals, "/usr/bin/mongod")
	c.Check(version, gc.Equals, mongo.Version{
		Major:         3,
		Minor:         4,
		Patch:         "14",
		StorageEngine: mongo.WiredTiger,
	})
}

func (s *MongodFinderSuite) TestStatButNoExecSystemMongo(c *gc.C) {
	s.setUpMock(c)
	defer s.ctrl.Finish()
	exp := s.search.EXPECT()
	exp.Stat("/usr/bin/mongod").Return(cannedFileInfo{}, nil)
	exp.RunCommand("/usr/bin/mongod", "--version").Return(
		"bad result", &exec.ExitError{},
	)
	path, version, err := s.finder.FindBest()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Check(path, gc.Equals, "")
	c.Check(version, gc.Equals, mongo.Version{})
}

func (s *MongodFinderSuite) TestParseMongoVersion(c *gc.C) {
	assertVersion := func(major, minor int, patch string, content string) {
		v, err := mongo.ParseMongoVersion(content)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(v.Major, gc.Equals, major)
		c.Check(v.Minor, gc.Equals, minor)
		c.Check(v.Patch, gc.Equals, patch)
	}
	assertVersion(3, 4, "14", mongodb34Version)
	assertVersion(3, 6, "3", mongodb36Version)
	assertVersion(3, 4, "0-rc3", "db version v3.4.0-rc3\n")
	assertVersion(2, 4, "6", "db version v2.4.6\n")
	assertVersion(2, 4, "", "db version v2.4.")
}

func (s *MongodFinderSuite) TestParseBadVersion(c *gc.C) {
	assertError := func(errMatch string, content string) {
		v, err := mongo.ParseMongoVersion(content)
		c.Check(err, gc.ErrorMatches, errMatch)
		c.Check(v, gc.Equals, mongo.Version{})
	}
	assertError("could not determine mongo version from:\nbad string\n", "bad string\n")
}

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
