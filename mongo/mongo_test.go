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
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/mongo"
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
	AptGetBase []string
}{
	AptGetBase: []string{
		"--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::Options::=--force-unsafe-io",
		"--assume-yes",
		"--quiet",
		"install",
	},
}

func makeEnsureServerParams(dataDir string) mongo.EnsureServerParams {
	return mongo.EnsureServerParams{
		StatePort:    testInfo.StatePort,
		Cert:         testInfo.Cert,
		PrivateKey:   testInfo.PrivateKey,
		SharedSecret: testInfo.SharedSecret,

		DataDir: dataDir,
	}
}

func (s *MongoSuite) makeConfigArgs(dataDir string) mongo.ConfigArgs {
	return mongo.ConfigArgs{
		DataDir:     dataDir,
		DBDir:       dataDir,
		MongoPath:   mongo.JujuMongod24Path,
		Port:        1234,
		OplogSizeMB: 1024,
		WantNUMACtl: false,
		Version:     s.mongodVersion,
		IPv6:        true,
	}
}

func (s *MongoSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.mongodVersion = mongo.Mongo24

	testing.PatchExecutable(c, s, "mongod", "#!/bin/bash\n\nprintf %s 'db version v2.4.9'\n")
	jujuMongodPath, err := exec.LookPath("mongod")
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(&mongo.JujuMongod24Path, jujuMongodPath)
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
	s.PatchValue(mongo.SmallOplogSizeMB, 1)

	testPath := c.MkDir()
	s.mongodConfigPath = filepath.Join(testPath, "mongodConfig")
	s.PatchValue(mongo.MongoConfigPath, s.mongodConfigPath)

	s.data = svctesting.NewFakeServiceData()
	mongo.PatchService(s.PatchValue, s.data)
}

func (s *MongoSuite) patchSeries(ser string) {
	s.PatchValue(&series.MustHostSeries, func() string { return ser })
}

func (s *MongoSuite) TestJujuMongodPath(c *gc.C) {
	obtained, err := mongo.Path(s.mongodVersion)
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtained, gc.Matches, s.mongodPath)
}

func (s *MongoSuite) TestDefaultMongodPath(c *gc.C) {
	s.PatchValue(&mongo.JujuMongod24Path, "/not/going/to/exist/mongod")
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
	dataDir := s.testEnsureServerNUMACtl(c, false)

	s.assertSSLKeyFile(c, dataDir)
	s.assertSharedSecretFile(c, dataDir)
	s.assertMongoConfigFile(c)

	// make sure that we log the version of mongodb as we get ready to
	// start it
	tlog := c.GetTestLog()
	any := `(.|\n)*`
	start := "^" + any
	tail := any + "$"
	c.Assert(tlog, gc.Matches, start+`using mongod: .*mongod --version: "db version v\d\.\d\.\d`+tail)
}

func (s *MongoSuite) TestEnsureServerServerExistsAndRunning(c *gc.C) {
	dataDir := c.MkDir()

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)

	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	s.data.SetStatus(mongo.ServiceName, "running")
	s.data.SetErrors(nil, nil, nil, errors.New("shouldn't be called"))

	_, err = mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Assert(err, jc.ErrorIsNil)

	// These should still be written out even if the service was installed.
	s.assertSSLKeyFile(c, dataDir)
	s.assertSharedSecretFile(c, dataDir)
	s.assertMongoConfigFile(c)

	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running")
}

func (s *MongoSuite) TestEnsureServerSetsSysctlValues(c *gc.C) {
	dataDir := c.MkDir()
	dataFilePath := filepath.Join(dataDir, "mongoKernelTweaks")
	dataFile, err := os.Create(dataFilePath)
	c.Assert(err, jc.ErrorIsNil)
	_, err = dataFile.WriteString("original value")
	c.Assert(err, jc.ErrorIsNil)
	dataFile.Close()

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	s.data.SetStatus(mongo.ServiceName, "running")

	contents, err := ioutil.ReadFile(dataFilePath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, "original value")

	_, err = mongo.SysctlEditableEnsureServer(makeEnsureServerParams(dataDir),
		map[string]string{dataFilePath: "new value"})
	c.Assert(err, jc.ErrorIsNil)

	contents, err = ioutil.ReadFile(dataFilePath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, "new value")
}

func (s *MongoSuite) TestEnsureServerServerExistsNotRunningIsStarted(c *gc.C) {
	dataDir := c.MkDir()

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	s.data.SetStatus(mongo.ServiceName, "installed")

	_, err = mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Assert(err, jc.ErrorIsNil)

	// These should still be written out even if the service was installed.
	s.assertSSLKeyFile(c, dataDir)
	s.assertSharedSecretFile(c, dataDir)
	s.assertMongoConfigFile(c)

	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running", "Start")
}

func (s *MongoSuite) TestEnsureServerServerExistsDifferentIsRewritten(c *gc.C) {
	dataDir := c.MkDir()

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	s.data.SetStatus(mongo.ServiceName, "different")

	_, err = mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Assert(err, jc.ErrorIsNil)

	// These should still be written out even if the service was installed.
	s.assertSSLKeyFile(c, dataDir)
	s.assertSharedSecretFile(c, dataDir)
	s.assertMongoConfigFile(c)

	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Stop", "Install", "Stop", "Start")
}

func (s *MongoSuite) TestEnsureServerServerExistsNotRunningStartError(c *gc.C) {
	dataDir := c.MkDir()

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	s.data.SetStatus(mongo.ServiceName, "installed")
	failure := errors.New("won't start")
	s.data.SetErrors(
		nil,     // Installed
		nil,     // Exists
		nil,     // Running
		failure, // Start
	)

	_, err = mongo.EnsureServer(makeEnsureServerParams(dataDir))

	c.Check(errors.Cause(err), gc.Equals, failure)
	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running", "Start")
}

func (s *MongoSuite) TestEnsureServerNUMACtl(c *gc.C) {
	s.testEnsureServerNUMACtl(c, true)
}

func (s *MongoSuite) testEnsureServerNUMACtl(c *gc.C, setNUMAPolicy bool) string {
	dataDir := c.MkDir()
	dbDir := filepath.Join(dataDir, "db")

	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	testParams := makeEnsureServerParams(dataDir)
	testParams.SetNUMAControlPolicy = setNUMAPolicy
	_, err = mongo.EnsureServer(testParams)
	c.Assert(err, jc.ErrorIsNil)

	testJournalDirs(dbDir, c)

	assertInstalled := func() {
		installed := s.data.Installed()
		c.Assert(installed, gc.HasLen, 1)
		service := installed[0]
		c.Assert(service.Name(), gc.Equals, "juju-db")
		c.Assert(service.Conf().Desc, gc.Equals, "juju state database")
		if setNUMAPolicy {
			stripped := strings.Replace(service.Conf().ExtraScript, "\n", "", -1)
			c.Assert(stripped, gc.Matches, `.* sysctl .*`)
		} else {
			c.Assert(service.Conf().ExtraScript, gc.Equals, "")
		}
		c.Assert(service.Conf().ExecStart, gc.Matches, `.*mongod.*`)
		c.Assert(service.Conf().Logfile, gc.Equals, "")
	}
	assertInstalled()
	return dataDir
}

func (s *MongoSuite) TestInstallMongodOnUbuntuViaApt(c *gc.C) {
	type installs struct {
		series  string
		pkgs    []string
		expOpts []string
	}

	tests := []installs{
		{"trusty", []string{"juju-mongodb"}, nil},
		{"xenial", []string{"juju-mongodb3.2", "juju-mongo-tools3.2"}, nil},
		{"bionic", []string{"mongodb-server-core", "mongodb-clients"}, nil},
	}

	dataDir := c.MkDir()
	for _, test := range tests {
		c.Logf("Testing mongo install for series: %s", test.series)
		testing.PatchExecutableAsEchoArgs(c, s, "apt-get")
		s.patchSeries(test.series)

		_, err := mongo.EnsureServer(makeEnsureServerParams(dataDir))
		c.Assert(err, gc.IsNil)

		for _, pkg := range test.pkgs {
			exp := append([]string{}, expectedArgs.AptGetBase...)
			exp = append(exp, test.expOpts...)
			exp = append(exp, pkg)
			testing.AssertEchoArgs(c, "apt-get", exp...)
		}
	}
}

func (s *MongoSuite) TestMongoAptGetFails(c *gc.C) {
	s.assertTestMongoGetFails(c, "trusty", "apt-get")
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
	writer := loggo.NewMinimumLevelWriter(&tw, loggo.ERROR)
	c.Assert(loggo.RegisterWriter("test-writer", writer), jc.ErrorIsNil)
	defer loggo.RemoveWriter("test-writer")

	dataDir := c.MkDir()
	_, err := mongo.EnsureServer(makeEnsureServerParams(dataDir))

	// Even though apt-get failed, EnsureServer should continue and
	// not return the error - even though apt-get failed, the Juju
	// mongodb package is most likely already installed.
	// The error should be logged however.
	c.Assert(err, jc.ErrorIsNil)

	c.Check(tw.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.ERROR, `packaging command failed: .+`},
		{loggo.ERROR, `cannot install/upgrade mongod \(will proceed anyway\): installing package.+: packaging command failed:.+`},
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

	_, err = mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.data.Installed(), gc.HasLen, 0)
	s.data.CheckCallNames(c, "Installed", "Exists", "Running")
}

func (s *MongoSuite) TestNewServiceWithReplSet(c *gc.C) {
	confArgs := s.makeConfigArgs(c.MkDir())
	conf := mongo.NewConf(&confArgs)
	c.Assert(strings.Contains(conf.ExecStart, "--replSet"), jc.IsTrue)
}

func (s *MongoSuite) TestNewServiceWithNumCtl(c *gc.C) {
	args := s.makeConfigArgs(c.MkDir())
	args.WantNUMACtl = true
	conf := mongo.NewConf(&args)
	c.Assert(conf.ExtraScript, gc.Not(gc.Matches), "")
}

func (s *MongoSuite) TestNewServiceWithIPv6(c *gc.C) {
	args := s.makeConfigArgs(c.MkDir())
	args.IPv6 = true
	conf := mongo.NewConf(&args)
	c.Assert(strings.Contains(conf.ExecStart, "--ipv6"), jc.IsTrue)
}

func (s *MongoSuite) TestNewServiceWithoutIPv6(c *gc.C) {
	args := s.makeConfigArgs(c.MkDir())
	args.IPv6 = false
	conf := mongo.NewConf(&args)
	c.Assert(strings.Contains(conf.ExecStart, "--ipv6"), jc.IsFalse)
}

func (s *MongoSuite) TestNewServiceWithJournal(c *gc.C) {
	args := s.makeConfigArgs(c.MkDir())
	args.Journal = true
	conf := mongo.NewConf(&args)
	c.Assert(conf.ExecStart, gc.Matches, `.* --journal.*`)
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

func (s *MongoSuite) TestNoMongoDir(c *gc.C) {
	// Make a non-existent directory that can nonetheless be
	// created.
	pm, err := coretesting.GetPackageManager()
	c.Assert(err, jc.ErrorIsNil)
	testing.PatchExecutableAsEchoArgs(c, s, pm.PackageManager)

	dataDir := filepath.Join(c.MkDir(), "dir", "data")
	_, err = mongo.EnsureServer(makeEnsureServerParams(dataDir))
	c.Check(err, jc.ErrorIsNil)

	_, err = os.Stat(filepath.Join(dataDir, "db"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MongoSuite) TestSelectPeerAddress(c *gc.C) {
	addresses := network.ProviderAddresses{
		network.NewScopedProviderAddress("126.0.0.1", network.ScopeMachineLocal),
		network.NewScopedProviderAddress("10.0.0.1", network.ScopeCloudLocal),
		network.NewScopedProviderAddress("8.8.8.8", network.ScopePublic),
	}

	address := mongo.SelectPeerAddress(addresses)
	c.Assert(address, gc.Equals, "10.0.0.1")
}

func (s *MongoSuite) TestGenerateSharedSecret(c *gc.C) {
	secret, err := mongo.GenerateSharedSecret()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secret, gc.HasLen, 1024)
	_, err = base64.StdEncoding.DecodeString(secret)
	c.Assert(err, jc.ErrorIsNil)
}

// failCmd creates an executable file at the given location that will do nothing
// except return an error.
func failCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 1"), 0755)
	if err != nil {
		panic(err)
	}
}
