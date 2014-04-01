package mongo

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

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
	// mock out the upstart commands so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))

	s.PatchValue(&upstart.InitDir, c.MkDir())
}

func fakeCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 0"), 0755)
	if err != nil {
		panic(err)
	}
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
	namespace := "user-local"

	// Make fake old service.
	// We defer the removes manually just in case the test fails, we don't leave
	// junk behind.
	conf := makeService(ServiceName(namespace), c)

	// now that the old service is installed, change the in-memory expected
	// script to something different.
	conf.Cmd = "echo something else"
	defer conf.Remove()

	// this should remove the old service, because the contents now differ
	err := removeOldService(conf)
	c.Check(err, gc.IsNil)

	c.Check(conf.Installed(), jc.IsFalse)
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
	namespace := "namespace"
	oldsvc := makeService(ServiceName(namespace), c)
	defer oldsvc.StopAndRemove()

	err := EnsureMongoServer(dir, port, namespace)
	c.Assert(err, gc.IsNil)
	mongodPath := MongodPathForSeries("some-series")
	svc, err := MongoUpstartService(namespace, mongodPath, dir, port)
	c.Assert(err, gc.IsNil)
	defer svc.StopAndRemove()

	testJournalDirs(dir, c)
	c.Assert(svc.Installed(), jc.IsTrue)
	conf, err := svc.ReadConf()
	c.Assert(err, gc.IsNil)
	expected, err := svc.Render()
	c.Assert(err, gc.IsNil)
	c.Assert(string(conf), gc.Equals, string(expected))

	// now check we can call it multiple times without error
	err = EnsureMongoServer(dir, port, namespace)
	c.Assert(err, gc.IsNil)
	c.Assert(svc.Installed(), jc.IsTrue)
	conf, err = svc.ReadConf()
	c.Assert(err, gc.IsNil)
	expected, err = svc.Render()
	c.Assert(err, gc.IsNil)
	c.Assert(string(conf), gc.Equals, string(expected))
}

func (s *MongoSuite) TestServiceName(c *gc.C) {
	name := ServiceName("foo")
	c.Assert(name, gc.Equals, "juju-db-foo")
	name = ServiceName("")
	c.Assert(name, gc.Equals, "juju-db")
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
