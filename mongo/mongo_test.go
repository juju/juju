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

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/packaging/manager"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type MongoSuite struct {
	coretesting.BaseSuite
	mongodConfigPath string
	mongodPath       string

	data *svctesting.FakeServiceData
}

var _ = gc.Suite(&MongoSuite{})

var testInfo = struct {
	StatePort    int
	Cert         string
	PrivateKey   string
	SharedSecret string
}{
	StatePort:    25252,
	Cert:         "foobar-cert",
	PrivateKey:   "foobar-privkey",
	SharedSecret: "foobar-sharedsecret",
}

func makeEnsureServerParams(dataDir, namespace string) mongo.EnsureServerParams {
	return mongo.EnsureServerParams{
		StatePort:    testInfo.StatePort,
		Cert:         testInfo.Cert,
		PrivateKey:   testInfo.PrivateKey,
		SharedSecret: testInfo.SharedSecret,

		DataDir:   dataDir,
		Namespace: namespace,
	}
}

func (s *MongoSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	// Try to make sure we don't execute any commands accidentally.
	s.PatchEnvironment("PATH", "")

	s.mongodPath = filepath.Join(c.MkDir(), "mongod")
	err := ioutil.WriteFile(s.mongodPath, []byte("#!/bin/bash\n\nprintf %s 'db version v2.4.9'\n"), 0755)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&mongo.JujuMongodPath, s.mongodPath)

	// Patch "df" such that it always reports there's 1MB free.
	s.PatchValue(mongo.AvailSpace, func(dir string) (float64, error) {
		info, err := os.Stat(dir)
		if err != nil {
			return 0, err
		}
		if info.IsDir() {
			return 1, nil

		}
		return 0, fmt.Errorf("not a directory")
	})
	s.PatchValue(mongo.MinOplogSizeMB, 1)

	testPath := c.MkDir()
	s.mongodConfigPath = filepath.Join(testPath, "mongodConfig")
	s.PatchValue(mongo.MongoConfigPath, s.mongodConfigPath)

	s.data = svctesting.NewFakeServiceData()
	mongo.PatchService(s.PatchValue, s.data)
}

func (s *MongoSuite) TestJujuMongodPath(c *gc.C) {
	obtained, err := mongo.Path()
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtained, gc.Equals, s.mongodPath)
}

func (s *MongoSuite) TestDefaultMongodPath(c *gc.C) {
	s.PatchValue(&mongo.JujuMongodPath, "/not/going/to/exist/mongod")
	s.PatchEnvPathPrepend(filepath.Dir(s.mongodPath))

	obtained, err := mongo.Path()
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtained, gc.Equals, s.mongodPath)
}

func (s *MongoSuite) TestMakeJournalDirs(c *gc.C) {
	dir := c.MkDir()
	err := mongo.MakeJournalDirs(dir)
	c.Assert(err, jc.ErrorIsNil)

	testJournalDirs(dir, c)
}

func testJournalDirs(dir string, c *gc.C) {
	journalDir := path.Join(dir, "journal")

	c.Assert(journalDir, jc.IsDirectory)
	info, err := os.Stat(filepath.Join(journalDir, "prealloc.0"))
	c.Assert(err, jc.ErrorIsNil)

	size := int64(1024 * 1024)

	c.Assert(info.Size(), gc.Equals, size)
	info, err = os.Stat(filepath.Join(journalDir, "prealloc.1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Size(), gc.Equals, size)
	info, err = os.Stat(filepath.Join(journalDir, "prealloc.2"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Size(), gc.Equals, size)
}

func (s *MongoSuite) assertSSLKeyFile(c *gc.C, dataDir string) {
	contents, err := ioutil.ReadFile(mongo.SSLKeyPath(dataDir))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, testInfo.Cert+"\n"+testInfo.PrivateKey)
}

func (s *MongoSuite) assertSharedSecretFile(c *gc.C, dataDir string) {
	contents, err := ioutil.ReadFile(mongo.SharedSecretPath(dataDir))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, testInfo.SharedSecret)
}

func (s *MongoSuite) assertMongoConfigFile(c *gc.C) {
	contents, err := ioutil.ReadFile(s.mongodConfigPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(contents, jc.DeepEquals, []byte("ENABLE_MONGODB=no"))
}

func (s *MongoSuite) TestEnsureServer(c *gc.C) {
	dataDir := s.testEnsureServerNumaCtl(c, false)

	s.assertSSLKeyFile(c, dataDir)
	s.assertSharedSecretFile(c, dataDir)
	s.assertMongoConfigFile(c)

	// make sure that we log the version of mongodb as we get ready to
	// start it
	tlog := c.GetTestLog()
	any := `(.|\n)*`
	start := "^" + any
	tail := any + "$"
	c.Assert(tlog, gc.Matches, start+`using mongod: .*/mongod --version: "db version v2\.4\.9`+tail)
}

func (s *MongoSuite) TestEnsureServerServerExistsAndRunning(c *gc.C) {
	dataDir := c.MkDir()
	namespace := "namespace"

	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	s.data.SetStatus(mongo.ServiceName(namespace), "running")
	s.data.SetErrors(nil, nil, nil, errors.New("shouldn't be called"))

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))
	c.Assert(err, jc.ErrorIsNil)

	// These should still be written out even if the service was installed.
	s.assertSSLKeyFile(c, dataDir)
	s.assertSharedSecretFile(c, dataDir)
	s.assertMongoConfigFile(c)

	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running")
}

func (s *MongoSuite) TestEnsureServerServerExistsNotRunningIsStarted(c *gc.C) {
	dataDir := c.MkDir()
	namespace := "namespace"

	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	s.data.SetStatus(mongo.ServiceName(namespace), "installed")

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))
	c.Assert(err, jc.ErrorIsNil)

	// These should still be written out even if the service was installed.
	s.assertSSLKeyFile(c, dataDir)
	s.assertSharedSecretFile(c, dataDir)
	s.assertMongoConfigFile(c)

	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running", "Start")
}

func (s *MongoSuite) TestEnsureServerServerExistsNotRunningStartError(c *gc.C) {
	dataDir := c.MkDir()
	namespace := "namespace"

	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	s.data.SetStatus(mongo.ServiceName(namespace), "installed")
	failure := errors.New("won't start")
	s.data.SetErrors(nil, nil, nil, failure) // Installed, Exists, Running, Running, Start

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))

	c.Check(errors.Cause(err), gc.Equals, failure)
	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running", "Start")
}

func (s *MongoSuite) TestEnsureServerNumaCtl(c *gc.C) {
	s.testEnsureServerNumaCtl(c, true)
}

func (s *MongoSuite) testEnsureServerNumaCtl(c *gc.C, setNumaPolicy bool) string {
	dataDir := c.MkDir()
	dbDir := filepath.Join(dataDir, "db")
	namespace := "namespace"

	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	testParams := makeEnsureServerParams(dataDir, namespace)
	testParams.SetNumaControlPolicy = setNumaPolicy
	err := mongo.EnsureServer(testParams)
	c.Assert(err, jc.ErrorIsNil)

	testJournalDirs(dbDir, c)

	assertInstalled := func() {
		installed := s.data.Installed()
		c.Assert(installed, gc.HasLen, 1)
		service := installed[0]
		c.Assert(service.Name(), gc.Equals, "juju-db-namespace")
		c.Assert(service.Conf().Desc, gc.Equals, "juju state database")
		if setNumaPolicy {
			stripped := strings.Replace(service.Conf().ExtraScript, "\n", "", -1)
			c.Assert(stripped, gc.Matches, `.* sysctl .*`)
		} else {
			c.Assert(service.Conf().ExtraScript, gc.Equals, "")
		}
		c.Assert(service.Conf().ExecStart, gc.Matches, ".*"+regexp.QuoteMeta(s.mongodPath)+".*")
		c.Assert(service.Conf().Logfile, gc.Equals, "")
	}
	assertInstalled()
	return dataDir
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

		err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))
		c.Assert(err, jc.ErrorIsNil)

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

func (s *MongoSuite) TestMongoAptGetFails(c *gc.C) {
	s.PatchValue(&version.Current.Series, "trusty")

	// Any exit code from apt-get that isn't 0 or 100 will be treated
	// as unexpected, skipping the normal retry loop. failCmd causes
	// the command to exit with 1.
	binDir := c.MkDir()
	s.PatchEnvPathPrepend(binDir)
	failCmd(filepath.Join(binDir, "apt-get"))

	// Set the mongodb service as installed but not running.
	namespace := "namespace"
	s.data.SetStatus(mongo.ServiceName(namespace), "installed")

	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("test-writer", &tw, loggo.ERROR), jc.ErrorIsNil)
	defer loggo.RemoveWriter("test-writer")

	dataDir := c.MkDir()
	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))

	// Even though apt-get failed, EnsureServer should continue and
	// not return the error - even though apt-get failed, the Juju
	// mongodb package is most likely already installed.
	// The error should be logged however.
	c.Assert(err, jc.ErrorIsNil)

	c.Check(tw.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.ERROR, `packaging command failed: .+`},
		{loggo.ERROR, `cannot install/upgrade mongod \(will proceed anyway\): packaging command failed`},
	})

	// Verify that EnsureServer continued and started the mongodb service.
	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running", "Start")
}

func (s *MongoSuite) TestInstallMongodServiceExists(c *gc.C) {
	output := mockShellCommand(c, &s.CleanupSuite, "apt-get")
	dataDir := c.MkDir()
	namespace := "namespace"

	s.data.SetStatus(mongo.ServiceName(namespace), "running")
	s.data.SetErrors(nil, nil, nil, errors.New("shouldn't be called"))

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running")

	// We still attempt to install mongodb, despite the service existing.
	cmds := getMockShellCalls(c, output)
	c.Check(cmds, gc.HasLen, 1)
}

func (s *MongoSuite) TestNewServiceWithReplSet(c *gc.C) {
	dataDir := c.MkDir()

	conf := mongo.NewConf(dataDir, dataDir, mongo.JujuMongodPath, 1234, 1024, false)
	c.Assert(strings.Contains(conf.ExecStart, "--replSet"), jc.IsTrue)
}

func (s *MongoSuite) TestNewServiceWithNumCtl(c *gc.C) {
	dataDir := c.MkDir()

	conf := mongo.NewConf(dataDir, dataDir, mongo.JujuMongodPath, 1234, 1024, true)
	c.Assert(conf.ExtraScript, gc.Not(gc.Matches), "")
}

func (s *MongoSuite) TestNewServiceIPv6(c *gc.C) {
	dataDir := c.MkDir()

	conf := mongo.NewConf(dataDir, dataDir, mongo.JujuMongodPath, 1234, 1024, false)
	c.Assert(strings.Contains(conf.ExecStart, "--ipv6"), jc.IsTrue)
}

func (s *MongoSuite) TestNewServiceWithJournal(c *gc.C) {
	dataDir := c.MkDir()

	conf := mongo.NewConf(dataDir, dataDir, mongo.JujuMongodPath, 1234, 1024, false)
	c.Assert(conf.ExecStart, gc.Matches, `.* --journal.*`)
}

func (s *MongoSuite) TestNoAuthCommandWithJournal(c *gc.C) {
	dataDir := c.MkDir()

	cmd, err := mongo.NoauthCommand(dataDir, 1234)
	c.Assert(err, jc.ErrorIsNil)
	var isJournalPresent bool
	for _, value := range cmd.Args {
		if value == "--journal" {
			isJournalPresent = true
		}
	}
	c.Assert(isJournalPresent, jc.IsTrue)
}

func (s *MongoSuite) TestRemoveService(c *gc.C) {
	namespace := "namespace"
	s.data.SetStatus(mongo.ServiceName(namespace), "running")

	err := mongo.RemoveService(namespace)
	c.Assert(err, jc.ErrorIsNil)

	removed := s.data.Removed()
	if !c.Check(removed, gc.HasLen, 1) {
		c.Check(removed[0].Name(), gc.Equals, "juju-db-namespace")
		c.Check(removed[0].Conf(), jc.DeepEquals, common.Conf{})
	}
	s.data.CheckCallNames(c, "Stop", "Remove")
}

func (s *MongoSuite) TestQuantalAptAddRepo(c *gc.C) {
	dir := c.MkDir()
	// patch manager.RunCommandWithRetry for repository addition:
	s.PatchValue(&manager.RunCommandWithRetry, func(string) (string, int, error) {
		return "", 1, fmt.Errorf("packaging command failed: exit status 1")
	})
	s.PatchEnvPathPrepend(dir)
	failCmd(filepath.Join(dir, "add-apt-repository"))
	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("test-writer", &tw, loggo.ERROR), jc.ErrorIsNil)
	defer loggo.RemoveWriter("test-writer")

	// test that we call add-apt-repository only for quantal
	// (and that if it fails, we log the error)
	s.PatchValue(&version.Current.Series, "quantal")
	err := mongo.EnsureServer(makeEnsureServerParams(dir, ""))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(tw.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.ERROR, `cannot install/upgrade mongod \(will proceed anyway\): packaging command failed`},
	})

	s.PatchValue(&manager.RunCommandWithRetry, func(string) (string, int, error) {
		return "", 0, nil
	})
	s.PatchValue(&version.Current.Series, "trusty")
	failCmd(filepath.Join(dir, "mongod"))
	err = mongo.EnsureServer(makeEnsureServerParams(dir, ""))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MongoSuite) TestNoMongoDir(c *gc.C) {
	// Make a non-existent directory that can nonetheless be
	// created.
	mockShellCommand(c, &s.CleanupSuite, "apt-get")
	dataDir := filepath.Join(c.MkDir(), "dir", "data")
	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, ""))
	c.Check(err, jc.ErrorIsNil)

	_, err = os.Stat(filepath.Join(dataDir, "db"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MongoSuite) TestServiceName(c *gc.C) {
	name := mongo.ServiceName("foo")
	c.Assert(name, gc.Equals, "juju-db-foo")
	name = mongo.ServiceName("")
	c.Assert(name, gc.Equals, "juju-db")
}

func (s *MongoSuite) TestSelectPeerAddress(c *gc.C) {
	addresses := []network.Address{{
		Value:       "10.0.0.1",
		Type:        network.IPv4Address,
		NetworkName: "cloud",
		Scope:       network.ScopeCloudLocal}, {
		Value:       "8.8.8.8",
		Type:        network.IPv4Address,
		NetworkName: "public",
		Scope:       network.ScopePublic}}

	address := mongo.SelectPeerAddress(addresses)
	c.Assert(address, gc.Equals, "10.0.0.1")
}

func (s *MongoSuite) TestSelectPeerHostPort(c *gc.C) {

	hostPorts := []network.HostPort{{
		Address: network.Address{
			Value:       "10.0.0.1",
			Type:        network.IPv4Address,
			NetworkName: "cloud",
			Scope:       network.ScopeCloudLocal,
		},
		Port: 37017}, {
		Address: network.Address{
			Value:       "8.8.8.8",
			Type:        network.IPv4Address,
			NetworkName: "public",
			Scope:       network.ScopePublic,
		},
		Port: 37017}}

	address := mongo.SelectPeerHostPort(hostPorts)
	c.Assert(address, gc.Equals, "10.0.0.1:37017")
}

func (s *MongoSuite) TestGenerateSharedSecret(c *gc.C) {
	secret, err := mongo.GenerateSharedSecret()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret, gc.HasLen, 1024)
	_, err = base64.StdEncoding.DecodeString(secret)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MongoSuite) TestAddPPAInQuantal(c *gc.C) {
	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	addAptRepoOut := mockShellCommand(c, &s.CleanupSuite, "add-apt-repository")
	s.PatchValue(&version.Current.Series, "quantal")

	dataDir := c.MkDir()
	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, ""))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(getMockShellCalls(c, addAptRepoOut), gc.DeepEquals, [][]string{{
		"--yes",
		"\"ppa:juju/stable\"",
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
