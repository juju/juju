// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	stdtesting "testing"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent/mongo"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

func Test(t *stdtesting.T) { gc.TestingT(t) }

type MongoSuite struct {
	coretesting.BaseSuite
	mongodConfigPath string
	mongodPath       string

	installError error
	installed    []upstart.Conf

	removeError error
	removed     []upstart.Service
}

var _ = gc.Suite(&MongoSuite{})

var testInfo = params.StateServingInfo{
	StatePort:    25252,
	Cert:         "foobar-cert",
	PrivateKey:   "foobar-privkey",
	SharedSecret: "foobar-sharedsecret",
}

func (s *MongoSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	// Try to make sure we don't execute any commands accidentally.
	s.PatchEnvironment("PATH", "")

	s.mongodPath = filepath.Join(c.MkDir(), "mongod")
	err := ioutil.WriteFile(s.mongodPath, []byte("#!/bin/bash\n\nprintf %s 'db version v2.4.9'\n"), 0755)
	c.Assert(err, gc.IsNil)
	s.PatchValue(&mongo.JujuMongodPath, s.mongodPath)

	testPath := c.MkDir()
	s.mongodConfigPath = filepath.Join(testPath, "mongodConfig")
	s.PatchValue(mongo.MongoConfigPath, s.mongodConfigPath)

	s.PatchValue(mongo.UpstartConfInstall, func(conf *upstart.Conf) error {
		s.installed = append(s.installed, *conf)
		return s.installError
	})
	s.PatchValue(mongo.UpstartServiceStopAndRemove, func(svc *upstart.Service) error {
		s.removed = append(s.removed, *svc)
		return s.removeError
	})
	// Clear out the values that are set by the above patched functions.
	s.removeError = nil
	s.installError = nil
	s.installed = nil
	s.removed = nil
}

func (s *MongoSuite) TestJujuMongodPath(c *gc.C) {
	obtained, err := mongo.Path()
	c.Check(err, gc.IsNil)
	c.Check(obtained, gc.Equals, s.mongodPath)
}

func (s *MongoSuite) TestDefaultMongodPath(c *gc.C) {
	s.PatchValue(&mongo.JujuMongodPath, "/not/going/to/exist/mongod")
	s.PatchEnvPathPrepend(filepath.Dir(s.mongodPath))

	obtained, err := mongo.Path()
	c.Check(err, gc.IsNil)
	c.Check(obtained, gc.Equals, s.mongodPath)
}

func (s *MongoSuite) TestMakeJournalDirs(c *gc.C) {
	dir := c.MkDir()
	err := mongo.MakeJournalDirs(dir)
	c.Assert(err, gc.IsNil)

	testJournalDirs(dir, c)
}

func testJournalDirs(dir string, c *gc.C) {
	journalDir := path.Join(dir, "journal")

	c.Assert(journalDir, jc.IsDirectory)
	info, err := os.Stat(filepath.Join(journalDir, "prealloc.0"))
	c.Assert(err, gc.IsNil)

	size := int64(1024 * 1024)

	c.Assert(info.Size(), gc.Equals, size)
	info, err = os.Stat(filepath.Join(journalDir, "prealloc.1"))
	c.Assert(err, gc.IsNil)
	c.Assert(info.Size(), gc.Equals, size)
	info, err = os.Stat(filepath.Join(journalDir, "prealloc.2"))
	c.Assert(err, gc.IsNil)
	c.Assert(info.Size(), gc.Equals, size)
}

func (s *MongoSuite) TestEnsureServer(c *gc.C) {
	dataDir := c.MkDir()
	dbDir := filepath.Join(dataDir, "db")
	namespace := "namespace"

	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	err := mongo.EnsureServer(dataDir, namespace, testInfo)
	c.Assert(err, gc.IsNil)

	testJournalDirs(dbDir, c)

	assertInstalled := func() {
		c.Assert(s.installed, gc.HasLen, 1)
		conf := s.installed[0]
		c.Assert(conf.Name, gc.Equals, "juju-db-namespace")
		c.Assert(conf.InitDir, gc.Equals, "/etc/init")
		c.Assert(conf.Desc, gc.Equals, "juju state database")
		c.Assert(conf.Cmd, gc.Matches, regexp.QuoteMeta(s.mongodPath)+".*")
		// TODO(nate) set Out so that mongod output goes somewhere useful?
		c.Assert(conf.Out, gc.Equals, "")
	}
	assertInstalled()

	contents, err := ioutil.ReadFile(s.mongodConfigPath)
	c.Assert(err, gc.IsNil)
	c.Assert(contents, jc.DeepEquals, []byte("ENABLE_MONGODB=no"))

	contents, err = ioutil.ReadFile(mongo.SSLKeyPath(dataDir))
	c.Assert(err, gc.IsNil)
	c.Assert(string(contents), gc.Equals, testInfo.Cert+"\n"+testInfo.PrivateKey)

	contents, err = ioutil.ReadFile(mongo.SharedSecretPath(dataDir))
	c.Assert(err, gc.IsNil)
	c.Assert(string(contents), gc.Equals, testInfo.SharedSecret)

	s.installed = nil
	// now check we can call it multiple times without error
	err = mongo.EnsureServer(dataDir, namespace, testInfo)
	c.Assert(err, gc.IsNil)
	assertInstalled()

	// make sure that we log the version of mongodb as we get ready to
	// start it
	tlog := c.GetTestLog()
	any := `(.|\n)*`
	start := "^" + any
	tail := any + "$"
	c.Assert(tlog, gc.Matches, start+`using mongod: .*/mongod --version: "db version v2\.4\.9`+tail)
}

func (s *MongoSuite) TestInstallMongod(c *gc.C) {
	type installs struct {
		series string
		pkg    string
	}
	tests := []installs{
		{"precise", "mongodb-server"},
		{"quantal", "mongodb-server"},
		{"raring", "mongodb-server"},
		{"saucy", "mongodb-server"},
		{"trusty", "juju-mongodb"},
		{"u-series", "juju-mongodb"},
	}

	mockShellCommand(c, &s.CleanupSuite, "add-apt-repository")
	output := mockShellCommand(c, &s.CleanupSuite, "apt-get")
	for _, test := range tests {
		c.Logf("Testing %s", test.series)
		dataDir := c.MkDir()
		namespace := "namespace" + test.series

		s.PatchValue(&version.Current.Series, test.series)

		err := mongo.EnsureServer(dataDir, namespace, testInfo)
		c.Assert(err, gc.IsNil)

		cmds := getMockShellCalls(c, output)

		// quantal does an extra apt-get install for python software properties
		// so we need to remember to skip that one
		index := 0
		if test.series == "quantal" {
			index = 1
		}
		match := fmt.Sprintf(`.* install .*%s`, test.pkg)
		c.Assert(strings.Join(cmds[index], " "), gc.Matches, match)
		// remove the temp file between tests
		c.Assert(os.Remove(output), gc.IsNil)
	}
}

func (s *MongoSuite) TestUpstartService(c *gc.C) {
	dataDir := c.MkDir()

	svc, _, err := mongo.UpstartService("", dataDir, dataDir, 1234)
	c.Assert(err, gc.IsNil)
	c.Assert(strings.Contains(svc.Cmd, "--replSet"), jc.IsTrue)
}

func (s *MongoSuite) TestUpstartServiceWithJournal(c *gc.C) {
	dataDir := c.MkDir()

	svc, _, err := mongo.UpstartService("", dataDir, dataDir, 1234)
	c.Assert(err, gc.IsNil)
	journalPresent := strings.Contains(svc.Cmd, " --journal ") || strings.HasSuffix(svc.Cmd, " --journal")
	c.Assert(journalPresent, jc.IsTrue)
}

func (s *MongoSuite) TestNoAuthCommandWithJournal(c *gc.C) {
	dataDir := c.MkDir()

	cmd, err := mongo.NoauthCommand(dataDir, 1234)
	c.Assert(err, gc.IsNil)
	var isJournalPresent bool
	for _, value := range cmd.Args {
		if value == "--journal" {
			isJournalPresent = true
		}
	}
	c.Assert(isJournalPresent, jc.IsTrue)
}

func (s *MongoSuite) TestRemoveService(c *gc.C) {
	err := mongo.RemoveService("namespace")
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
	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	// test that we call add-apt-repository only for quantal (and that if it
	// fails, we return the error)
	s.PatchValue(&version.Current.Series, "quantal")
	err := mongo.EnsureServer(dir, "", testInfo)
	c.Assert(err, gc.ErrorMatches, "cannot install mongod: cannot add apt repository: exit status 1.*")

	s.PatchValue(&version.Current.Series, "trusty")
	err = mongo.EnsureServer(dir, "", testInfo)
	c.Assert(err, gc.IsNil)
}

func (s *MongoSuite) TestNoMongoDir(c *gc.C) {
	// Make a non-existent directory that can nonetheless be
	// created.
	mockShellCommand(c, &s.CleanupSuite, "apt-get")
	dataDir := filepath.Join(c.MkDir(), "dir", "data")
	err := mongo.EnsureServer(dataDir, "", testInfo)
	c.Check(err, gc.IsNil)

	_, err = os.Stat(filepath.Join(dataDir, "db"))
	c.Assert(err, gc.IsNil)
}

func (s *MongoSuite) TestServiceName(c *gc.C) {
	name := mongo.ServiceName("foo")
	c.Assert(name, gc.Equals, "juju-db-foo")
	name = mongo.ServiceName("")
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

	address := mongo.SelectPeerAddress(addresses)
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

	address := mongo.SelectPeerHostPort(hostPorts)
	c.Assert(address, gc.Equals, "10.0.0.1:37017")
}

func (s *MongoSuite) TestGenerateSharedSecret(c *gc.C) {
	secret, err := mongo.GenerateSharedSecret()
	c.Assert(err, gc.IsNil)
	c.Assert(secret, gc.HasLen, 1024)
	_, err = base64.StdEncoding.DecodeString(secret)
	c.Assert(err, gc.IsNil)
}

func (s *MongoSuite) TestAddPPAInQuantal(c *gc.C) {
	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	addAptRepoOut := mockShellCommand(c, &s.CleanupSuite, "add-apt-repository")
	s.PatchValue(&version.Current.Series, "quantal")

	dataDir := c.MkDir()
	err := mongo.EnsureServer(dataDir, "", testInfo)
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
func mockShellCommand(c *gc.C, s *testing.CleanupSuite, name string) string {
	dir := c.MkDir()
	s.PatchEnvPathPrepend(dir)

	// Note the shell script produces output of the form:
	// +arg1+\n
	// +arg2+\n
	// ...
	// +argn+\n
	// -
	//
	// It would be nice if there was a simple way of unambiguously
	// quoting shell arguments, but this will do as long
	// as no argument contains a newline character.
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

// getMockShellCalls, given a file name returned by mockShellCommands,
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

// splitCall splits the output produced by a single call to the
// mocked shell function (see mockShellCommand) and
// splits it into its individual arguments.
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

// failCmd creates an executable file at the given location that will do nothing
// except return an error.
func failCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 1"), 0755)
	if err != nil {
		panic(err)
	}
}
