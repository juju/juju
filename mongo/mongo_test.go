// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/packaging/manager"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	environs "github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
	coretesting "github.com/juju/juju/testing"
)

type MongoSuite struct {
	coretesting.BaseSuite
	mongodConfigPath string
	mongodPath       string
	mongodVersion    mongo.Version

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

var expectedArgs = struct {
	MongoInstall []jc.SimpleMessage
	YumBase      []string
	AptGetBase   []string
	Semanage     []string
	Chcon        []string
}{
	MongoInstall: []jc.SimpleMessage{
		{loggo.INFO, "Ensuring mongo server is running; data directory.*"},
		{loggo.INFO, "Running: yum --assumeyes --debuglevel=1 install epel-release"},
		{loggo.INFO, regexp.QuoteMeta("installing [mongodb-server]")},
		{loggo.INFO, "Running: yum --assumeyes --debuglevel=1 install mongodb-server"},
	},
	YumBase: []string{
		"--assumeyes",
		"--debuglevel=1",
		"install",
	},
	AptGetBase: []string{
		"--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io",
		"--assume-yes",
		"--quiet",
		"install",
	},
	Semanage: []string{
		"port",
		"-a",
		"-t",
		"mongod_port_t",
		"-p",
		"tcp",
		strconv.Itoa(environs.DefaultStatePort),
	},
	Chcon: []string{
		"-R",
		"-v",
		"-t",
		"mongod_var_lib_t",
		"/var/lib/juju/",
	},
}

func makeEnsureServerParams(dataDir string) mongo.EnsureServerParams {
	return mongo.EnsureServerParams{
		StatePort:    testInfo.StatePort,
		Cert:         testInfo.Cert,
		PrivateKey:   testInfo.PrivateKey,
		SharedSecret: testInfo.SharedSecret,

		DataDir: dataDir,
		Version: mongo.Mongo24,
	}
}

func (s *MongoSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.mongodVersion = mongo.Mongo24

	testing.PatchExecutable(c, s, "mongod", "#!/bin/bash\n\nprintf %s 'db version v2.4.9'\n")
	jujuMongodPath, err := exec.LookPath("mongod")
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(&mongo.JujuMongodPath, jujuMongodPath)
	s.mongodPath = jujuMongodPath

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

func (s *MongoSuite) patchSeries(ser string) {
	s.PatchValue(&series.HostSeries, func() string { return ser })
}

func (s *MongoSuite) TestJujuMongodPath(c *gc.C) {
	obtained, err := mongo.Path(s.mongodVersion)
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtained, gc.Matches, s.mongodPath)
}

func (s *MongoSuite) TestDefaultMongodPath(c *gc.C) {
	s.PatchValue(&mongo.JujuMongodPath, "/not/going/to/exist/mongod")
	s.PatchEnvPathPrepend(filepath.Dir(s.mongodPath))

	c.Logf("mongo version is %q", s.mongodVersion)
	obtained, err := mongo.Path(s.mongodVersion)
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtained, gc.Matches, s.mongodPath)
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

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)

	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	s.data.SetStatus(mongo.ServiceName, "running")
	s.data.SetErrors(nil, nil, nil, errors.New("shouldn't be called"))

	err = mongo.EnsureServer(makeEnsureServerParams(dataDir))
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

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	s.data.SetStatus(mongo.ServiceName, "installed")

	err = mongo.EnsureServer(makeEnsureServerParams(dataDir))
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

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	s.data.SetStatus(mongo.ServiceName, "installed")
	failure := errors.New("won't start")
	s.data.SetErrors(nil, nil, nil, failure) // Installed, Exists, Running, Running, Start

	err = mongo.EnsureServer(makeEnsureServerParams(dataDir))

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

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	testParams := makeEnsureServerParams(dataDir)
	testParams.SetNumaControlPolicy = setNumaPolicy
	err = mongo.EnsureServer(testParams)
	c.Assert(err, jc.ErrorIsNil)

	testJournalDirs(dbDir, c)

	assertInstalled := func() {
		installed := s.data.Installed()
		c.Assert(installed, gc.HasLen, 1)
		service := installed[0]
		c.Assert(service.Name(), gc.Equals, "juju-db")
		c.Assert(service.Conf().Desc, gc.Equals, "juju state database")
		if setNumaPolicy {
			stripped := strings.Replace(service.Conf().ExtraScript, "\n", "", -1)
			c.Assert(stripped, gc.Matches, `.* sysctl .*`)
		} else {
			c.Assert(service.Conf().ExtraScript, gc.Equals, "")
		}
		c.Assert(service.Conf().ExecStart, gc.Matches, `.*/check-.*/mongod.*`)
		c.Assert(service.Conf().Logfile, gc.Equals, "")
	}
	assertInstalled()
	return dataDir
}

func (s *MongoSuite) TestInstallMongod(c *gc.C) {
	type installs struct {
		series string
		cmd    [][]string
	}

	tests := []installs{
		{"precise", [][]string{{"--target-release", "precise-updates/cloud-tools", "mongodb-server"}}},
		{"quantal", [][]string{{"python-software-properties"}, {"--target-release", "mongodb-server"}}},
		{"raring", [][]string{{"--target-release", "mongodb-server"}}},
		{"saucy", [][]string{{"--target-release", "mongodb-server"}}},
		{"trusty", [][]string{{"juju-mongodb"}}},
		{"u-series", [][]string{{"juju-mongodb"}}},
	}

	testing.PatchExecutableAsEchoArgs(c, s, "add-apt-repository")
	testing.PatchExecutableAsEchoArgs(c, s, "apt-get")
	for _, test := range tests {
		dataDir := c.MkDir()
		s.patchSeries(test.series)
		err := mongo.EnsureServer(makeEnsureServerParams(dataDir))
		c.Assert(err, jc.ErrorIsNil)

		for _, cmd := range test.cmd {
			match := append(expectedArgs.AptGetBase, cmd...)
			testing.AssertEchoArgs(c, "apt-get", match...)
		}
	}
}

func (s *MongoSuite) TestInstallFailChconMongodCentOS(c *gc.C) {
	returnCode := 1
	execNameFail := "chcon"

	exec := []string{"yum", "chcon"}

	expectedResult := append(expectedArgs.MongoInstall, []jc.SimpleMessage{
		{loggo.INFO, "running " + execNameFail + " .*"},
		{loggo.ERROR, execNameFail + " failed to change file security context error exit status " + strconv.Itoa(returnCode)},
		{loggo.ERROR, regexp.QuoteMeta("cannot install/upgrade mongod (will proceed anyway): exit status " + strconv.Itoa(returnCode))},
	}...)
	s.assertSuccessWithInstallStepFailCentOS(c, exec, execNameFail, returnCode, expectedResult)
}

func (s *MongoSuite) TestSemanageRuleExistsDoesNotFail(c *gc.C) {
	// if the return code is 1 then the rule already exists and we do not fail
	returnCode := 1
	execNameFail := "semanage"

	exec := []string{"yum", "chcon"}

	expectedResult := append(expectedArgs.MongoInstall, []jc.SimpleMessage{
		{loggo.INFO, "running chcon .*"},
		{loggo.INFO, "running " + execNameFail + " .*"},
	}...)

	s.assertSuccessWithInstallStepFailCentOS(c, exec, execNameFail, returnCode, expectedResult)
}

func (s *MongoSuite) TestInstallFailSemanageMongodCentOS(c *gc.C) {
	returnCode := 2
	execNameFail := "semanage"

	exec := []string{"yum", "chcon"}

	expectedResult := append(expectedArgs.MongoInstall, []jc.SimpleMessage{
		{loggo.INFO, "running chcon .*"},
		{loggo.INFO, "running " + execNameFail + " .*"},
		{loggo.ERROR, execNameFail + " failed to provide access on port " + strconv.Itoa(environs.DefaultStatePort) + " error exit status " + strconv.Itoa(returnCode)},
		{loggo.ERROR, regexp.QuoteMeta("cannot install/upgrade mongod (will proceed anyway): exit status " + strconv.Itoa(returnCode))},
	}...)
	s.assertSuccessWithInstallStepFailCentOS(c, exec, execNameFail, returnCode, expectedResult)
}

func (s *MongoSuite) assertSuccessWithInstallStepFailCentOS(c *gc.C, exec []string, execNameFail string, returnCode int, expectedResult []jc.SimpleMessage) {
	type installs struct {
		series string
		pkg    string
	}
	test := installs{
		"centos7", "mongodb*",
	}

	for _, e := range exec {
		testing.PatchExecutableAsEchoArgs(c, s, e)
	}

	testing.PatchExecutableThrowError(c, s, execNameFail, returnCode)

	dataDir := c.MkDir()
	s.patchSeries(test.series)

	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("mongosuite", &tw, loggo.INFO), jc.ErrorIsNil)
	defer loggo.RemoveWriter("mongosuite")

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, expectedResult)
}

func (s *MongoSuite) TestInstallSuccessMongodCentOS(c *gc.C) {
	type installs struct {
		series string
		pkg    string
	}
	test := installs{
		"centos7", "mongodb*",
	}

	testing.PatchExecutableAsEchoArgs(c, s, "yum")
	testing.PatchExecutableAsEchoArgs(c, s, "chcon")
	testing.PatchExecutableAsEchoArgs(c, s, "semanage")

	dataDir := c.MkDir()
	s.patchSeries(test.series)

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Assert(err, jc.ErrorIsNil)

	expected := append(expectedArgs.YumBase, "epel-release")

	testing.AssertEchoArgs(c, "yum", expected...)

	testing.AssertEchoArgs(c, "chcon", expectedArgs.Chcon...)

	testing.AssertEchoArgs(c, "semanage", expectedArgs.Semanage...)
}

func (s *MongoSuite) TestMongoAptGetFails(c *gc.C) {
	s.assertTestMongoGetFails(c, "trusty", "apt-get")
}

func (s *MongoSuite) TestMongoYumFails(c *gc.C) {
	s.assertTestMongoGetFails(c, "centos7", "yum")
}

func (s *MongoSuite) assertTestMongoGetFails(c *gc.C, series string, packageManager string) {
	s.patchSeries(series)

	// Any exit code from apt-get that isn't 0 or 100 will be treated
	// as unexpected, skipping the normal retry loop. failCmd causes
	// the command to exit with 1.
	binDir := c.MkDir()
	s.PatchEnvPathPrepend(binDir)
	failCmd(filepath.Join(binDir, packageManager))

	// Set the mongodb service as installed but not running.
	s.data.SetStatus(mongo.ServiceName, "installed")

	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("test-writer", &tw, loggo.ERROR), jc.ErrorIsNil)
	defer loggo.RemoveWriter("test-writer")

	dataDir := c.MkDir()
	err := mongo.EnsureServer(makeEnsureServerParams(dataDir))

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
	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)
	if pm.PackageManager == "yum" {
		testing.PatchExecutableAsEchoArgs(c, s, "chcon")
		testing.PatchExecutableAsEchoArgs(c, s, "semanage")
	}

	dataDir := c.MkDir()

	s.data.SetStatus(mongo.ServiceName, "running")
	s.data.SetErrors(nil, nil, nil, errors.New("shouldn't be called"))

	err = mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running")
}

func (s *MongoSuite) TestNewServiceWithReplSet(c *gc.C) {
	dataDir := c.MkDir()

	conf := mongo.NewConf(dataDir, dataDir, mongo.JujuMongodPath, 1234, 1024, false, s.mongodVersion, true)
	c.Assert(strings.Contains(conf.ExecStart, "--replSet"), jc.IsTrue)
}

func (s *MongoSuite) TestNewServiceWithNumCtl(c *gc.C) {
	dataDir := c.MkDir()

	conf := mongo.NewConf(dataDir, dataDir, mongo.JujuMongodPath, 1234, 1024, true, s.mongodVersion, true)
	c.Assert(conf.ExtraScript, gc.Not(gc.Matches), "")
}

func (s *MongoSuite) TestNewServiceIPv6(c *gc.C) {
	dataDir := c.MkDir()

	conf := mongo.NewConf(dataDir, dataDir, mongo.JujuMongodPath, 1234, 1024, false, s.mongodVersion, true)
	c.Assert(strings.Contains(conf.ExecStart, "--ipv6"), jc.IsTrue)
}

func (s *MongoSuite) TestNewServiceWithJournal(c *gc.C) {
	dataDir := c.MkDir()

	conf := mongo.NewConf(dataDir, dataDir, mongo.JujuMongodPath, 1234, 1024, false, s.mongodVersion, true)
	c.Assert(conf.ExecStart, gc.Matches, `.* --journal.*`)
}

func (s *MongoSuite) TestNoAuthCommandWithJournal(c *gc.C) {
	dataDir := c.MkDir()

	cmd, err := mongo.NoauthCommand(dataDir, 1234, s.mongodVersion)
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
	s.data.SetStatus(mongo.ServiceName, "running")

	err := mongo.RemoveService()
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

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	failCmd(filepath.Join(dir, pm.RepositoryManager))
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("test-writer", &tw, loggo.ERROR), jc.ErrorIsNil)
	defer loggo.RemoveWriter("test-writer")

	// test that we call add-apt-repository only for quantal
	// (and that if it fails, we log the error)
	s.patchSeries("quantal")
	err = mongo.EnsureServer(makeEnsureServerParams(dir))
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(tw.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.ERROR, `cannot install/upgrade mongod \(will proceed anyway\): packaging command failed`},
	})

	s.PatchValue(&manager.RunCommandWithRetry, func(string) (string, int, error) {
		return "", 0, nil
	})
	s.patchSeries("trusty")
	failCmd(filepath.Join(dir, "mongod"))
	err = mongo.EnsureServer(makeEnsureServerParams(dir))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MongoSuite) TestNoMongoDir(c *gc.C) {
	// Make a non-existent directory that can nonetheless be
	// created.
	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	dataDir := filepath.Join(c.MkDir(), "dir", "data")
	err = mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Check(err, jc.ErrorIsNil)

	_, err = os.Stat(filepath.Join(dataDir, "db"))
	c.Assert(err, jc.ErrorIsNil)
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
		Port: environs.DefaultStatePort}, {
		Address: network.Address{
			Value:       "8.8.8.8",
			Type:        network.IPv4Address,
			NetworkName: "public",
			Scope:       network.ScopePublic,
		},
		Port: environs.DefaultStatePort}}

	address := mongo.SelectPeerHostPort(hostPorts)
	c.Assert(address, gc.Equals, "10.0.0.1:"+strconv.Itoa(environs.DefaultStatePort))
}

func (s *MongoSuite) TestGenerateSharedSecret(c *gc.C) {
	secret, err := mongo.GenerateSharedSecret()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret, gc.HasLen, 1024)
	_, err = base64.StdEncoding.DecodeString(secret)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MongoSuite) TestAddPPAInQuantal(c *gc.C) {
	testing.PatchExecutableAsEchoArgs(c, s, "apt-get")

	testing.PatchExecutableAsEchoArgs(c, s, "add-apt-repository")
	s.patchSeries("quantal")

	dataDir := c.MkDir()
	err := mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Assert(err, jc.ErrorIsNil)

	pack := [][]string{
		{
			"install",
			"python-software-properties",
		}, {
			"install",
			"--target-release",
			"mongodb-server",
		},
	}
	noCommand := len(expectedArgs.AptGetBase) - 1
	for k := range pack {
		cmd := append(expectedArgs.AptGetBase[:noCommand], pack[k]...)
		testing.AssertEchoArgs(c, "apt-get", cmd...)
	}

	match := []string{
		"--yes",
		"\"ppa:juju/stable\"",
	}

	testing.AssertEchoArgs(c, "add-apt-repository", match...)
}

func (s *MongoSuite) TestAddEpelInCentOS(c *gc.C) {
	testing.PatchExecutableAsEchoArgs(c, s, "yum")

	s.patchSeries("centos7")

	testing.PatchExecutableAsEchoArgs(c, s, "chcon")
	testing.PatchExecutableAsEchoArgs(c, s, "semanage")
	testing.PatchExecutableAsEchoArgs(c, s, "yum-config-manager")

	dataDir := c.MkDir()
	err := mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Assert(err, jc.ErrorIsNil)

	expectedEpelRelease := append(expectedArgs.YumBase, "epel-release")
	testing.AssertEchoArgs(c, "yum", expectedEpelRelease...)

	expectedMongodbServer := append(expectedArgs.YumBase, "mongodb-server")
	testing.AssertEchoArgs(c, "yum", expectedMongodbServer...)

	testing.AssertEchoArgs(c, "chcon", expectedArgs.Chcon...)

	testing.AssertEchoArgs(c, "semanage", expectedArgs.Semanage...)
}

// failCmd creates an executable file at the given location that will do nothing
// except return an error.
func failCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 1"), 0755)
	if err != nil {
		panic(err)
	}
}
