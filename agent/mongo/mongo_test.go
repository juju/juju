package mongo

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

func Test(t *testing.T) { gc.TestingT(t) }

type MongoSuite struct {
	testbase.LoggingSuite
	mongodConfigPath string
	mongodPath       string

	installError error
	installed    []upstart.Conf

	removeError error
	removed     []upstart.Service
}

var (
	_        = gc.Suite(&MongoSuite{})
	testInfo = params.StateServingInfo{
		StatePort:    25252,
		Cert:         "foobar-cert",
		PrivateKey:   "foobar-privkey",
		SharedSecret: "foobar-sharedsecret",
	}
)

func (s *MongoSuite) SetUpTest(c *gc.C) {
	// Try to make sure we don't execute any commands accidentally.
	s.PatchEnvironment("PATH", "")

	s.mongodPath = filepath.Join(c.MkDir(), "mongod")
	err := ioutil.WriteFile(s.mongodPath, nil, 0755)
	c.Assert(err, gc.IsNil)
	s.PatchValue(&JujuMongodPath, s.mongodPath)

	testPath := c.MkDir()
	s.mongodConfigPath = filepath.Join(testPath, "mongodConfig")
	s.PatchValue(&mongoConfigPath, s.mongodConfigPath)

	s.PatchValue(&upstartConfInstall, func(conf *upstart.Conf) error {
		s.installed = append(s.installed, *conf)
		return s.installError
	})
	s.PatchValue(&upstartServiceStopAndRemove, func(svc *upstart.Service) error {
		s.removed = append(s.removed, *svc)
		return s.removeError
	})
	s.removeError = nil
	s.installError = nil
	s.installed = nil
	s.removed = nil
}

func (s *MongoSuite) TestJujuMongodPath(c *gc.C) {
	obtained, err := MongodPath()
	c.Check(err, gc.IsNil)
	c.Check(obtained, gc.Equals, s.mongodPath)
}

func (s *MongoSuite) TestDefaultMongodPath(c *gc.C) {
	s.PatchValue(&JujuMongodPath, "/not/going/to/exist/mongod")
	s.PatchEnvPathPrepend(filepath.Dir(s.mongodPath))

	obtained, err := MongodPath()
	c.Check(err, gc.IsNil)
	c.Check(obtained, gc.Equals, s.mongodPath)
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

	s.mockShellCommand(c, "apt-get")

	err := EnsureMongoServer(dataDir, namespace, testInfo)
	c.Assert(err, gc.IsNil)

	testJournalDirs(dbDir, c)

	c.Assert(s.installed, gc.HasLen, 1)
	conf := s.installed[0]
	c.Assert(conf.Name, gc.Equals, "juju-db-namespace")
	c.Assert(conf.InitDir, gc.Equals, "/etc/init")
	c.Assert(conf.Desc, gc.Not(gc.Equals), "")
	c.Assert(conf.Cmd, gc.Matches, regexp.QuoteMeta(s.mongodPath)+".*")
	// TODO set Out so that mongod output goes somewhere useful?
	c.Assert(conf.Out, gc.Equals, "")

	contents, err := ioutil.ReadFile(s.mongodConfigPath)
	c.Assert(err, gc.IsNil)
	c.Assert(contents, jc.DeepEquals, []byte("ENABLE_MONGODB=no"))

	contents, err = ioutil.ReadFile(sslKeyPath(dataDir))
	c.Assert(err, gc.IsNil)
	c.Assert(string(contents), gc.Equals, testInfo.Cert+"\n"+testInfo.PrivateKey)

	contents, err = ioutil.ReadFile(sharedSecretPath(dataDir))
	c.Assert(err, gc.IsNil)
	c.Assert(string(contents), gc.Equals, testInfo.SharedSecret)

	s.installed = nil
	// now check we can call it multiple times without error
	err = EnsureMongoServer(dataDir, namespace, testInfo)
	c.Assert(err, gc.IsNil)
	c.Assert(s.installed, gc.HasLen, 1)
}

func (s *MongoSuite) TestRemoveService(c *gc.C) {
	err := RemoveService("namespace")
	c.Assert(err, gc.IsNil)
	c.Assert(s.removed, jc.DeepEquals, []upstart.Service{{
		Name:    "juju-db-namespace",
		InitDir: upstart.InitDir,
	}})
}

func (s *MongoSuite) TestQuantalAptAddRepo(c *gc.C) {
	dir := c.MkDir()
	s.PatchEnvPathPrepend(dir)
	failCmd(filepath.Join(dir, "add-apt-repository"))
	s.mockShellCommand(c, "apt-get")

	// test that we call add-apt-repository only for quantal (and that if it
	// fails, we return the error)
	s.PatchValue(&version.Current.Series, "quantal")
	err := EnsureMongoServer(dir, "", testInfo)
	c.Assert(err, gc.ErrorMatches, "cannot install mongod: cannot add apt repository: exit status 1.*")

	s.PatchValue(&version.Current.Series, "trusty")
	err = EnsureMongoServer(dir, "", testInfo)
	c.Assert(err, gc.IsNil)
}

func (s *MongoSuite) TestNoMongoDir(c *gc.C) {
	// Make a non-existent directory that can nonetheless be
	// created.
	s.mockShellCommand(c, "apt-get")
	dataDir := filepath.Join(c.MkDir(), "dir", "data")
	err := EnsureMongoServer(dataDir, "", testInfo)
	c.Check(err, gc.IsNil)

	_, err = os.Stat(filepath.Join(dataDir, "db"))
	c.Assert(err, gc.IsNil)
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

func (s *MongoSuite) TestAddPPAInQuantal(c *gc.C) {
	s.mockShellCommand(c, "apt-get")

	addAptRepoOut := s.mockShellCommand(c, "add-apt-repository")
	s.PatchValue(&version.Current.Series, "quantal")

	dataDir := c.MkDir()
	err := EnsureMongoServer(dataDir, "", testInfo)
	c.Assert(err, gc.IsNil)

	c.Assert(getMockShellCalls(c, addAptRepoOut), gc.DeepEquals, [][]string{{
		"-y",
		"ppa:juju/stable",
	}})
}

// mockShellCommand creates a new command with the given
// name and contents, and patches $PATH so that it will be
// executed by preference. It returns the name of a file
// that is written by each call to the command - mockShellCalls
// can be used to retrieve the calls.
func (s *MongoSuite) mockShellCommand(c *gc.C, name string) string {
	dir := c.MkDir()
	s.PatchEnvPathPrepend(dir)

	outputFile := filepath.Join(dir, name+".out")
	contents := `#!/bin/sh
{
	for i in "$@"; do
		echo +"$i"+
	done
	echo -
} >> ` + utils.ShQuote(outputFile) + `
`
	err := ioutil.WriteFile(filepath.Join(dir, name), []byte(contents), 0755)
	c.Assert(err, gc.IsNil)
	return outputFile
}

// Given a file name returned by mockShellCommands, getMockShellCalls
// returns a slice containing one element for each call, each
// containing the arguments passed to the command.
// It will be confused if the arguments contain newlines.
func getMockShellCalls(c *gc.C, file string) [][]string {
	data, err := ioutil.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	c.Assert(err, gc.IsNil)
	s := string(data)
	parts := strings.Split(s, "\n-\n")
	c.Assert(parts[len(parts)-1], gc.Equals, "")
	var calls [][]string
	for _, part := range parts[0 : len(parts)-1] {
		calls = append(calls, splitCall(c, part))
	}
	return calls
}

func splitCall(c *gc.C, part string) []string {
	var result []string
	for _, arg := range strings.Split(part, "\n") {
		c.Assert(arg, gc.Matches, `\+.*\+`)
		arg = strings.TrimSuffix(arg, "+")
		arg = strings.TrimPrefix(arg, "+")
		result = append(result, arg)
	}
	return result
}

func failCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 1"), 0755)
	if err != nil {
		panic(err)
	}
}
