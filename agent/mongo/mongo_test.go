package mongo

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/upstart"
)

func Test(t *testing.T) { gc.TestingT(t) }

type MongoSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&MongoSuite{})

func (s *MongoSuite) SetUpSuite(c *gc.C) {
	testpath := c.MkDir()
	s.PatchEnvPathPrepend(testpath)
	// mock out the start method so we can fake install services without sudo
	start := filepath.Join(testpath, "start")
	err := ioutil.WriteFile(start, []byte("#!/bin/bash --norc\nexit 0"), 0755)
	c.Assert(err, gc.IsNil)

	s.PatchValue(&upstart.InitDir, c.MkDir())
}

func (s *MongoSuite) TestJujuMongodPath(c *gc.C) {
	d := c.MkDir()
	defer os.RemoveAll(d)
	mongoPath := filepath.Join(d, "mongod")
	s.PatchValue(&JujuMongodPath, mongoPath)

	err := ioutil.WriteFile(mongoPath, nil, 0777)
	c.Assert(err, gc.IsNil)

	obtained, err := MongodPath()
	c.Check(err, gc.IsNil)
	c.Check(obtained, gc.Equals, mongoPath)
}

func (s *MongoSuite) TestDefaultMongodPath(c *gc.C) {
	s.PatchValue(&JujuMongodPath, "/not/going/to/exist/mongod")

	dir := c.MkDir()
	s.PatchEnvPathPrepend(dir)
	filename := filepath.Join(dir, "mongod")
	err := ioutil.WriteFile(filename, nil, 0777)
	c.Assert(err, gc.IsNil)

	obtained, err := MongodPath()
	c.Check(err, gc.IsNil)
	c.Check(obtained, gc.Equals, filename)
}

func (s *MongoSuite) TestRemoveOldMongoServices(c *gc.C) {
	s.PatchValue(&oldMongoServiceName, "someNameThatShouldntExist")

	// Make fake old services.
	// We defer the removes manually just in case the test fails, we don't leave
	// junk behind.
	conf := makeService(oldMongoServiceName, c)
	defer conf.Remove()
	conf2 := makeService(makeServiceName(2), c)
	defer conf2.Remove()
	conf3 := makeService(makeServiceName(3), c)
	defer conf3.Remove()

	// Remove with current version = 4, which should remove all previous
	// versions plus the old service name.
	err := removeOldMongoServices(4)
	c.Assert(err, gc.IsNil)

	c.Assert(conf.Installed(), jc.IsFalse)
	c.Assert(conf2.Installed(), jc.IsFalse)
	c.Assert(conf3.Installed(), jc.IsFalse)
}

func (s *MongoSuite) TestMakeJournalDirs(c *gc.C) {
	dir := c.MkDir()
	err := makeJournalDirs(dir)
	c.Assert(err, gc.IsNil)

	testJournalDirs(dir, c)
}

func testJournalDirs(dir string, c *gc.C) {
	journalDir := path.Join(dir, "journal")

	c.Check(journalDir, jc.IsDirectory)
	info, err := os.Stat(filepath.Join(journalDir, "prealloc.0"))
	c.Check(err, gc.IsNil)

	size := int64(1024 * 1024)

	c.Check(info.Size(), gc.Equals, size)
	info, err = os.Stat(filepath.Join(journalDir, "prealloc.1"))
	c.Check(err, gc.IsNil)
	c.Check(info.Size(), gc.Equals, size)
	info, err = os.Stat(filepath.Join(journalDir, "prealloc.2"))
	c.Check(err, gc.IsNil)
	c.Check(info.Size(), gc.Equals, size)

}

func (s *MongoSuite) TestEnsureMongoServer(c *gc.C) {
	dir := c.MkDir()
	port := 25252

	oldsvc := makeService(oldMongoServiceName, c)
	defer oldsvc.Remove()

	err := EnsureMongoServer(dir, port)
	c.Assert(err, gc.IsNil)
	svc, err := MongoUpstartService(makeServiceName(mongoScriptVersion), dir, port)
	c.Assert(err, gc.IsNil)
	defer svc.Remove()

	testJournalDirs(dir, c)
	c.Check(oldsvc.Installed(), jc.IsFalse)
	c.Check(svc.Installed(), jc.IsTrue)

	// now check we can call it multiple times without error
	err = EnsureMongoServer(dir, port)
	c.Assert(err, gc.IsNil)

}

func makeService(name string, c *gc.C) *upstart.Conf {
	conf := &upstart.Conf{
		Desc:    "foo",
		Service: *upstart.NewService(name),
		Cmd:     "echo hi",
	}
	err := conf.Install()
	c.Assert(err, gc.IsNil)
	return conf
}
