// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package mongo

import (
	"errors"
	"os"
	"testing"
	"time"

	coretesting "github.com/juju/juju/testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type MongoVersionSuite struct {
	coretesting.BaseSuite
}

type MongoPathSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&MongoVersionSuite{})
var _ = gc.Suite(&MongoPathSuite{})

func Test(t *testing.T) {
	gc.TestingT(t)
}

var (
	version1   = Version{Major: 1, Minor: 0}
	version1d1 = Version{Major: 1, Minor: 1}
	version2   = Version{Major: 2, Minor: 0}
)

func (m *MongoVersionSuite) TestVersionNewerCompareMinors(c *gc.C) {
	res := version1.NewerThan(version1d1)
	c.Assert(res, gc.Equals, -1)
	res = version1d1.NewerThan(version1)
	c.Assert(res, gc.Equals, 1)
	res = version1.NewerThan(version1)
	c.Assert(res, gc.Equals, 0)
}

func (m *MongoVersionSuite) TestVersionNewerCompareMayors(c *gc.C) {
	res := version1.NewerThan(version2)
	c.Assert(res, gc.Equals, -1)
	res = version2.NewerThan(version1)
	c.Assert(res, gc.Equals, 1)
	res = version2.NewerThan(version1d1)
	c.Assert(res, gc.Equals, 1)
}

func (m *MongoVersionSuite) TestVersionNewerCompareSpecial(c *gc.C) {
	res := MongoUpgrade.NewerThan(version2)
	c.Assert(res, gc.Equals, 0)
	res = version2.NewerThan(MongoUpgrade)
	c.Assert(res, gc.Equals, 0)
}

func (m *MongoVersionSuite) TestString(c *gc.C) {
	s := version1.String()
	c.Assert(s, gc.Equals, "1.0")
	s = version1d1.String()
	c.Assert(s, gc.Equals, "1.1")
	v := Version{Major: 1, Minor: 2, Patch: "something"}
	s = v.String()
	c.Assert(s, gc.Equals, "1.2.something")
	v.StorageEngine = WiredTiger
	s = v.String()
	c.Assert(s, gc.Equals, "1.2.something/wiredTiger")
}

func (m *MongoPathSuite) TestMongodPath(c *gc.C) {
	pathTests := map[Version]string{
		Mongo26:   "/usr/lib/juju/mongo2.6/bin/mongod",
		Mongo32wt: "/usr/lib/juju/mongo3.2/bin/mongod",
	}
	for v, exp := range pathTests {
		p := JujuMongodPath(v)
		c.Assert(p, gc.Equals, exp)
	}
}

type fakeFileInfo struct {
	isDir bool
}

func (f fakeFileInfo) Name() string       { return "" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Now() }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() interface{}   { return nil }

func assertStat(c *gc.C, v Version, result bool, stat error, called string) {
	var statArg string
	statFunc := func(p string) (os.FileInfo, error) {
		statArg = p
		return fakeFileInfo{}, stat
	}

	b := binariesAvailable(v, statFunc)
	c.Assert(b, gc.Equals, result)
	c.Assert(statArg, gc.Equals, called)
}

func (m *MongoPathSuite) TestBinaries(c *gc.C) {
	errMissing := errors.New("missing")

	assertStat(c, Mongo24, true, nil, JujuMongod24Path)

	assertStat(c, Mongo24, false, errMissing, JujuMongod24Path)

	assertStat(c, Mongo32wt, true, nil, "/usr/lib/juju/mongo3.2/bin/mongod")

	assertStat(c, Mongo32wt, false, errMissing, "/usr/lib/juju/mongo3.2/bin/mongod")

	assertStat(c, Version{Major: 4, Minor: 3}, true, nil, "/usr/lib/juju/mongo4.3/bin/mongod")
}

func assertStatLook(c *gc.C, v Version, statErr, lookErr error, errReg, path, statCall, lookCall string) {
	var statCalled string
	stat := func(p string) (os.FileInfo, error) {
		statCalled = p
		return fakeFileInfo{}, statErr
	}
	var lookCalled string
	look := func(p string) (string, error) {
		lookCalled = p
		if lookErr != nil {
			return "", lookErr
		}
		return "/a/false/path", nil
	}
	p, err := mongoPath(v, stat, look)
	if errReg == "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, errReg)
	}

	c.Assert(p, gc.Equals, path)
	c.Assert(statCalled, gc.Equals, statCall)
	c.Assert(lookCalled, gc.Equals, lookCall)
}

func (m *MongoPathSuite) TestPath(c *gc.C) {
	errMissing := errors.New("missing")
	errLookupFailed := errors.New("failed lookup")
	assertStatLook(c, Mongo24, nil, nil, "", JujuMongod24Path, JujuMongod24Path, "")

	assertStatLook(c, Mongo24, errMissing, nil, "", "/a/false/path", JujuMongod24Path, "mongod")

	assertStatLook(c, Mongo24, errMissing, errLookupFailed, "*failed lookup", "", JujuMongod24Path, "mongod")

	mongo32Path := "/usr/lib/juju/mongo3.2/bin/mongod"
	assertStatLook(c, Mongo32wt, nil, nil, "", mongo32Path, mongo32Path, "")

	assertStatLook(c, Mongo32wt, errMissing, nil, "no suitable binary for \"3.2/wiredTiger\"", "", mongo32Path, "")
}
