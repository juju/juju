package mongo

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/version"
)

func Test(t *testing.T) { gc.TestingT(t) }

type MongoSuite struct {
	testbase.LoggingSuite
	mongodConfigPath string
}

var (
	_    = gc.Suite(&MongoSuite{})
	info = params.StateServingInfo{
		StatePort:    25252,
		Cert:         "foobar-cert",
		PrivateKey:   "foobar-privkey",
		SharedSecret: "foobar-sharedsecret",
	}
)

func (s *MongoSuite) SetUpSuite(c *gc.C) {
	testpath := c.MkDir()
	s.PatchEnvPathPrepend(testpath)
	// mock out the upstart commands so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))
	fakeCmd(filepath.Join(testpath, "apt-get"))

	s.PatchValue(&upstart.InitDir, c.MkDir())

	s.mongodConfigPath = filepath.Join(testpath, "mongodConfig")

	s.PatchValue(&mongoConfigPath, s.mongodConfigPath)
}

func fakeCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 0"), 0755)
	if err != nil {
		panic(err)
	}
}

func failCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 1"), 0755)
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
	dataDir := c.MkDir()
	dbDir := filepath.Join(dataDir, "db")
	namespace := "namespace"
	oldsvc := makeService(ServiceName(namespace), c)
	defer oldsvc.StopAndRemove()

	err := EnsureMongoServer(dataDir, namespace, info)
	c.Assert(err, gc.IsNil)
	svc, err := mongoUpstartService(namespace, dataDir, dbDir, info.StatePort)
	c.Assert(err, gc.IsNil)
	defer svc.StopAndRemove()

	testJournalDirs(dbDir, c)
	c.Assert(svc.Installed(), jc.IsTrue)

	contents, err := ioutil.ReadFile(s.mongodConfigPath)
	c.Assert(err, gc.IsNil)
	c.Assert(contents, jc.DeepEquals, []byte("ENABLE_MONGODB=no"))

	contents, err = ioutil.ReadFile(sslKeyPath(dataDir))
	c.Assert(err, gc.IsNil)
	c.Assert(string(contents), gc.Equals, info.Cert+"\n"+info.PrivateKey)

	contents, err = ioutil.ReadFile(sharedSecretPath(dataDir))
	c.Assert(err, gc.IsNil)
	c.Assert(string(contents), gc.Equals, info.SharedSecret)

	// now check we can call it multiple times without error
	err = EnsureMongoServer(dataDir, namespace, info)
	c.Assert(err, gc.IsNil)
	c.Assert(svc.Installed(), jc.IsTrue)
}

func (s *MongoSuite) TestQuantalAptAddRepo(c *gc.C) {
	dir := c.MkDir()
	s.PatchEnvPathPrepend(dir)
	failCmd(filepath.Join(dir, "add-apt-repository"))

	// test that we call add-apt-repository only for quantal (and that if it
	// fails, we return the error)
	s.PatchValue(&version.Current.Series, "quantal")
	err := EnsureMongoServer(dir, "", info)
	c.Assert(err, gc.ErrorMatches, "cannot install mongod: cannot add apt repository: exit status 1.*")

	s.PatchValue(&version.Current.Series, "trusty")
	err = EnsureMongoServer(dir, "", info)
	c.Assert(err, gc.IsNil)
}

func (s *MongoSuite) TestNoMongoDir(c *gc.C) {
	dataDir := c.MkDir()

	dbDir := filepath.Join(dataDir, "db")

	// remove the directory so we use the path but it won't exist
	// that should make it get cleaned up at the end of the test if created
	os.RemoveAll(dataDir)

	err := EnsureMongoServer(dataDir, "", info)
	c.Check(err, gc.IsNil)

	_, err = os.Stat(dbDir)
	c.Assert(err, gc.IsNil)

	svc, err := mongoUpstartService("", dataDir, dbDir, info.StatePort)
	c.Assert(err, gc.IsNil)
	defer svc.Remove()
}

func (s *MongoSuite) TestServiceName(c *gc.C) {
	name := ServiceName("foo")
	c.Assert(name, gc.Equals, "juju-db-foo")
	name = ServiceName("")
	c.Assert(name, gc.Equals, "juju-db")
}

func (s *MongoSuite) TestSelectPeerAddress(c *gc.C) {
	addresses := []instance.Address{{
		Value:        "10.0.0.1",
		Type:         instance.Ipv4Address,
		NetworkName:  "cloud",
		NetworkScope: instance.NetworkCloudLocal}, {
		Value:        "8.8.8.8",
		Type:         instance.Ipv4Address,
		NetworkName:  "public",
		NetworkScope: instance.NetworkPublic}}

	address := SelectPeerAddress(addresses)
	c.Assert(address, gc.Equals, "10.0.0.1")
}

func (s *MongoSuite) TestSelectPeerHostPort(c *gc.C) {

	hostPorts := []instance.HostPort{{
		Address: instance.Address{
			Value:        "10.0.0.1",
			Type:         instance.Ipv4Address,
			NetworkName:  "cloud",
			NetworkScope: instance.NetworkCloudLocal,
		},
		Port: 37017}, {
		Address: instance.Address{
			Value:        "8.8.8.8",
			Type:         instance.Ipv4Address,
			NetworkName:  "public",
			NetworkScope: instance.NetworkPublic,
		},
		Port: 37017}}

	address := SelectPeerHostPort(hostPorts)
	c.Assert(address, gc.Equals, "10.0.0.1:37017")
}

func (s *MongoSuite) TestGenerateSharedSecret(c *gc.C) {
	secret, err := GenerateSharedSecret()
	c.Assert(err, gc.IsNil)
	c.Assert(secret, gc.HasLen, 1024)
	_, err = base64.StdEncoding.DecodeString(secret)
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
