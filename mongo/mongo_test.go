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
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type MongoSuite struct {
	coretesting.BaseSuite
	mongodConfigPath string
	mongodPath       string
	installCount     int
	services         *svctesting.FakeServices
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
		StatePort: testInfo.StatePort,
		SSLInfo: mongo.SSLInfo{
			Cert:         testInfo.Cert,
			PrivateKey:   testInfo.PrivateKey,
			SharedSecret: testInfo.SharedSecret,
		},

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
	s.PatchValue(mongo.MongodPath, func() string {
		return s.mongodPath
	})

	// Patch "df" such that it always reports there's 1MB free.
	s.PatchValue(mongo.AvailSpace, func(dir string) (float64, error) {
		info, err := os.Stat(dir)
		if err != nil {
			return 0, err
		}
		if info.IsDir() {
			return 1, nil

		}
		return 0, errors.New("not a directory")
	})
	s.PatchValue(mongo.MinOplogSizeMB, 1)

	testPath := c.MkDir()
	s.mongodConfigPath = filepath.Join(testPath, "mongodConfig")
	s.PatchValue(mongo.MongoConfigPath, s.mongodConfigPath)

	s.services = svctesting.NewFakeServices(service.InitSystemUpstart)
	s.PatchValue(mongo.NewService, func(name, dataDir string, conf common.Conf) (*service.Service, error) {
		return service.WrapService(name, conf, s.services), nil
	})
	s.installCount = 0 // Reset for each pass.
	installService := *mongo.InstallService
	s.PatchValue(mongo.InstallService, func(svc *mongo.Service) error {
		s.installCount++
		return installService(svc)
	})
}

func (s *MongoSuite) setService(c *gc.C, namespace, status string) {
	name := mongo.ServiceName(namespace)

	err := s.services.Add(name, common.Conf{})
	c.Assert(err, jc.ErrorIsNil)

	switch status {
	case common.StatusRunning:
		err := s.services.Enable(name)
		c.Assert(err, jc.ErrorIsNil)
		err = s.services.Start(name)
		c.Assert(err, jc.ErrorIsNil)
	case common.StatusEnabled:
		err := s.services.Enable(name)
		c.Assert(err, jc.ErrorIsNil)
	}

	s.services.ResetCalls()
}

func (s *MongoSuite) enabled() []*service.Service {
	var enabled []*service.Service
	for _, name := range s.services.Status.Enabled.Values() {
		conf := s.services.Confs[name]
		svc := service.WrapService(name, conf, s.services)
		enabled = append(enabled, svc)
	}
	return enabled
}

func (s *MongoSuite) newServiceSpec(c *gc.C) mongo.ServiceSpec {
	dataDir := c.MkDir()

	return mongo.ServiceSpec{
		Executable:  (*mongo.MongodPath)(),
		DBDir:       dataDir,
		DataDir:     dataDir,
		Port:        1234,
		OplogSizeMB: 1024,
		WantNumaCtl: false,
	}
}

func (s *MongoSuite) TestJujuMongodPath(c *gc.C) {
	obtained, err := mongo.Path()
	c.Check(err, jc.ErrorIsNil)
	c.Check(obtained, gc.Equals, s.mongodPath)
}

func (s *MongoSuite) TestDefaultMongodPath(c *gc.C) {
	s.PatchValue(mongo.MongodPath, func() string {
		return "/not/going/to/exist/mongod"
	})
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

func (s *MongoSuite) TestEnsureServer(c *gc.C) {
	dataDir := s.testEnsureServerNumaCtl(c, false)

	contents, err := ioutil.ReadFile(s.mongodConfigPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(contents, jc.DeepEquals, []byte("ENABLE_MONGODB=no"))

	contents, err = ioutil.ReadFile(mongo.SSLKeyPath(dataDir))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, testInfo.Cert+"\n"+testInfo.PrivateKey)

	contents, err = ioutil.ReadFile(mongo.SharedSecretPath(dataDir))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(contents), gc.Equals, testInfo.SharedSecret)

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
	s.setService(c, namespace, common.StatusRunning)

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.installCount, gc.Equals, 0)
	s.services.CheckCalls(c, "Add", "IsEnabled", "Check", "IsRunning")
}

func (s *MongoSuite) TestEnsureServerServerExistsNotRunningIsStarted(c *gc.C) {
	dataDir := c.MkDir()
	namespace := "namespace"
	name := mongo.ServiceName(namespace)

	mockShellCommand(c, &s.CleanupSuite, "apt-get")
	s.setService(c, namespace, common.StatusEnabled)

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.installCount, gc.Equals, 0)
	s.services.CheckCalls(c, "Add", "IsEnabled", "Check", "IsRunning", "Start")

	running, err := s.services.IsRunning(name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(running, jc.IsTrue)
}

func (s *MongoSuite) TestEnsureServerServerExistsNotRunningStartError(c *gc.C) {
	dataDir := c.MkDir()
	namespace := "namespace"

	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	s.setService(c, namespace, common.StatusEnabled)
	failure := errors.New("won't start")
	// Add, IsEnabled, Check, IsRunning, **Start**
	s.services.Errors = []error{nil, nil, nil, nil, failure}

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))

	c.Check(err, gc.ErrorMatches, `.*won't start`)
	c.Check(s.installCount, gc.Equals, 0)
	s.services.CheckCalls(c, "Add", "IsEnabled", "Check", "IsRunning", "Start")
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
		c.Check(s.installCount, gc.Equals, 1)
		enabled := s.enabled()
		c.Assert(enabled, gc.HasLen, 1)
		service := enabled[0]
		conf := service.Conf()
		c.Check(service.Name(), gc.Equals, "juju-db-namespace")
		c.Check(conf.Desc, gc.Equals, "juju state database")
		if setNumaPolicy {
			stripped := strings.Replace(conf.ExtraScript, "\n", "", -1)
			c.Check(stripped, gc.Matches, `.* sysctl .*`)
		} else {
			c.Check(conf.ExtraScript, gc.Equals, "")
		}
		c.Check(conf.Cmd, gc.Matches, ".*"+regexp.QuoteMeta(s.mongodPath)+".*")
		// TODO(nate) set Out so that mongod output goes somewhere useful?
		c.Check(conf.Out, gc.Equals, "")
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

func (s *MongoSuite) TestInstallMongodServiceExists(c *gc.C) {
	output := mockShellCommand(c, &s.CleanupSuite, "apt-get")
	dataDir := c.MkDir()
	namespace := "namespace"

	s.setService(c, namespace, common.StatusRunning)
	failure := errors.New("shouldn't be called")
	// Add, IsEnabled, Check, IsRunning, **Start**
	s.services.Errors = []error{nil, nil, nil, nil, failure}

	err := mongo.EnsureServer(makeEnsureServerParams(dataDir, namespace))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(s.installCount, gc.Equals, 0)

	// We still attempt to install mongodb, despite the service existing.
	cmds := getMockShellCalls(c, output)
	c.Assert(cmds, gc.HasLen, 1)
}

func (s *MongoSuite) TestServiceConf(c *gc.C) {
	spec := s.newServiceSpec(c)
	conf := spec.Conf()

	c.Check(conf.ExtraScript, gc.Equals, "") // No Numa stuff.
	c.Check(conf.Cmd, gc.Matches, ".* --replSet .*")
	c.Check(conf.Cmd, gc.Matches, ".* --ipv6 .*")
	c.Check(conf.Cmd, gc.Matches, `.* --journal .*`)
}

func (s *MongoSuite) TestServiceConfWithNumCtl(c *gc.C) {
	spec := s.newServiceSpec(c)
	spec.WantNumaCtl = true
	conf := spec.Conf()

	c.Check(conf.ExtraScript, gc.Not(gc.Matches), "")
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

func (s *MongoSuite) TestQuantalAptAddRepo(c *gc.C) {
	dir := c.MkDir()
	s.PatchEnvPathPrepend(dir)
	failCmd(filepath.Join(dir, "add-apt-repository"))
	mockShellCommand(c, &s.CleanupSuite, "apt-get")

	// test that we call add-apt-repository only for quantal (and that if it
	// fails, we return the error)
	s.PatchValue(&version.Current.Series, "quantal")
	err := mongo.EnsureServer(makeEnsureServerParams(dir, ""))
	c.Assert(err, gc.ErrorMatches, "cannot install mongod: cannot add apt repository.*: exit status 1")

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
