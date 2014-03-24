package mongo

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"labix.org/v2/mgo"

	coretesting "launchpad.net/juju-core/testing"
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
	s.PatchValue(&initiateReplicaSet, func(EnsureMongoParams) error {
		return nil
	})

	dir := c.MkDir()
	address := "localhost"
	info := &mgo.DialInfo{}

	dbDir := filepath.Join(dir, "db")
	err := os.MkdirAll(dbDir, 0777)
	c.Assert(err, gc.IsNil)

	port := 25252
	hostPort := net.JoinHostPort(address, fmt.Sprint(port))

	oldsvc := makeService(oldMongoServiceName, c)
	defer oldsvc.Remove()

	err = EnsureMongoServer(EnsureMongoParams{
		HostPort: hostPort,
		DataDir:  dir,
		DialInfo: info,
	})
	c.Assert(err, gc.IsNil)
	svc, err := mongoUpstartService(makeServiceName(mongoScriptVersion), dir, dbDir, port)
	c.Assert(err, gc.IsNil)
	defer svc.Remove()

	testJournalDirs(dbDir, c)
	c.Check(oldsvc.Installed(), jc.IsFalse)
	c.Check(svc.Installed(), jc.IsTrue)

	// now check we can call it multiple times without error
	err = EnsureMongoServer(EnsureMongoParams{
		HostPort: hostPort,
		DataDir:  dir,
		DialInfo: info,
	})
	c.Assert(err, gc.IsNil)

}

func (s *MongoSuite) TestNoMongoDir(c *gc.C) {
	s.PatchValue(&initiateReplicaSet, func(EnsureMongoParams) error {
		return nil
	})

	dir := c.MkDir()
	address := "localhost"
	info := &mgo.DialInfo{}

	dbDir := filepath.Join(dir, "db")

	// remove the directory so we use the path but it won't exist
	// that should make it get cleaned up at the end of the test if created
	os.RemoveAll(dir)
	port := 25252
	hostPort := net.JoinHostPort(address, fmt.Sprint(port))

	err := EnsureMongoServer(EnsureMongoParams{
		HostPort: hostPort,
		DataDir:  dir,
		DialInfo: info,
	})
	c.Check(err, gc.IsNil)

	_, err = os.Stat(dbDir)
	c.Assert(err, gc.IsNil)

	svc, err := mongoUpstartService(makeServiceName(mongoScriptVersion), dir, dbDir, port)
	c.Assert(err, gc.IsNil)
	defer svc.Remove()
}

func (s *MongoSuite) TestInitiateReplicaSet(c *gc.C) {
	var err error
	inst := &coretesting.MgoInstance{Params: []string{"--replSet", "juju"}}
	err = inst.Start(true)
	c.Assert(err, gc.IsNil)

	info := inst.DialInfo()
	info.Direct = true

	// Set up the inital ReplicaSet
	err = initiateReplicaSet(EnsureMongoParams{
		HostPort: inst.Addr(),
		DialInfo: info,
	})
	c.Assert(err, gc.IsNil)

	// This would return a mgo.QueryError if a ReplicaSet
	// configuration already existed but we tried to created
	// one with replicaset.Initiate again.
	err = initiateReplicaSet(EnsureMongoParams{
		HostPort: inst.Addr(),
		DialInfo: info,
	})
	c.Assert(err, gc.IsNil)

	// TODO test login
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
