// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/names"
	"github.com/juju/replicaset"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type UpgradeMongoSuite struct{}
type UpgradeMongoCommandSuite struct{}

var _ = gc.Suite(&UpgradeMongoSuite{})
var _ = gc.Suite(&UpgradeMongoCommandSuite{})

type fakeFileInfo struct {
	isDir bool
}

func (f fakeFileInfo) Name() string       { return "" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Now() }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() interface{}   { return nil }

type fakeRunCommand struct {
	ranCommands [][]string
	mgoSession  mgoSession
	mgoDb       mgoDb
	service     service.Service
}

func (f *fakeRunCommand) runCommand(command string, args ...string) (string, error) {
	ran := []string{
		command,
	}
	ran = append(ran, args...)
	f.ranCommands = append(f.ranCommands, ran)
	return "", nil
}

func (f *fakeRunCommand) runCommandFail(command string, args ...string) (string, error) {
	ran := []string{
		command,
	}
	ran = append(ran, args...)
	f.ranCommands = append(f.ranCommands, ran)
	return "this failed", errors.New("a generic error")
}

func (f *fakeRunCommand) stat(statFile string) (os.FileInfo, error) {
	f.ranCommands = append(f.ranCommands, []string{"stat", statFile})
	return fakeFileInfo{}, nil
}

func (f *fakeRunCommand) remove(toremove string) error {
	f.ranCommands = append(f.ranCommands, []string{"remove", toremove})
	return nil
}

func (f *fakeRunCommand) mkdir(dirname string, mode os.FileMode) error {
	f.ranCommands = append(f.ranCommands, []string{"mkdir", dirname})
	return nil
}

func (f *fakeRunCommand) getenv(key string) string {
	f.ranCommands = append(f.ranCommands, []string{"getenv", key})
	return "bogus_daemon"
}

func (s *UpgradeMongoSuite) TestMongo26UpgradeStep(c *gc.C) {
	command := fakeRunCommand{}
	err := mongo26UpgradeStepCall(command.runCommand, "/a/fake/datadir")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ranCommands, gc.HasLen, 1)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/usr/lib/juju/bin/mongod", "--dbpath", "/a/fake/datadir/db", "--replSet", "juju", "--upgrade"})

	command = fakeRunCommand{}
	err = mongo26UpgradeStepCall(command.runCommandFail, "/a/fake/datadir")
	c.Assert(err, gc.ErrorMatches, "cannot upgrade mongo 2.4 data: a generic error")
}

func (s *UpgradeMongoSuite) TestRemoveOldDb(c *gc.C) {
	command := fakeRunCommand{}
	err := removeOldDbCall("/a/fake/datadir", command.stat, command.remove, command.mkdir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ranCommands, gc.HasLen, 3)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"stat", "/a/fake/datadir/db"})
	c.Assert(command.ranCommands[1], gc.DeepEquals, []string{"remove", "/a/fake/datadir/db"})
	c.Assert(command.ranCommands[2], gc.DeepEquals, []string{"mkdir", "/a/fake/datadir/db"})
}

func (s *UpgradeMongoSuite) TestMongoDump(c *gc.C) {
	command := fakeRunCommand{}
	out, err := mongoDumpCall(command.runCommand, "/fake/tmp/dir", "/fake/mongo/path", "adminpass", "aMigrationName", 1234)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "")
	c.Assert(command.ranCommands, gc.HasLen, 1)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/fake/mongo/path/mongodump", "--ssl", "-u", "admin", "-p", "adminpass", "--port", "1234", "--host", "localhost", "--out", "/fake/tmp/dir/migrateToaMigrationNamedump"})
}

func (s *UpgradeMongoSuite) TestMongoDumpRetries(c *gc.C) {
	command := fakeRunCommand{}
	out, err := mongoDumpCall(command.runCommandFail, "/fake/tmp/dir", "/fake/mongo/path", "", "aMigrationName", 1234)
	c.Assert(err, gc.ErrorMatches, "cannot dump mongo db: a generic error")
	c.Assert(out, gc.Equals, "this failed")
	c.Assert(command.ranCommands, gc.HasLen, 60)
	for i := range command.ranCommands {
		c.Logf("Checking attempt %d", i)
		c.Assert(command.ranCommands[i], gc.DeepEquals, []string{"/fake/mongo/path/mongodump", "--ssl", "-u", "admin", "-p", "", "--port", "1234", "--host", "localhost", "--out", "/fake/tmp/dir/migrateToaMigrationNamedump"})
	}
}

func (s *UpgradeMongoSuite) TestMongoRestore(c *gc.C) {
	command := fakeRunCommand{}
	err := mongoRestoreCall(command.runCommand, "/fake/tmp/dir", "/fake/mongo/path", "adminpass", "aMigrationName", []string{}, 1234, true, 100)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ranCommands, gc.HasLen, 1)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--port", "1234", "--host", "localhost", "--sslAllowInvalidCertificates", "--batchSize", "100", "-u", "admin", "-p", "adminpass", "/fake/tmp/dir/migrateToaMigrationNamedump"})
}

func (s *UpgradeMongoSuite) TestMongoRestoreWithoutAdmin(c *gc.C) {
	command := fakeRunCommand{}
	err := mongoRestoreCall(command.runCommand, "/fake/tmp/dir", "/fake/mongo/path", "", "aMigrationName", []string{}, 1234, false, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ranCommands, gc.HasLen, 1)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--port", "1234", "--host", "localhost", "/fake/tmp/dir/migrateToaMigrationNamedump"})
}

func (s *UpgradeMongoSuite) TestMongoRestoreWithDBs(c *gc.C) {
	command := fakeRunCommand{}
	err := mongoRestoreCall(command.runCommand, "/fake/tmp/dir", "/fake/mongo/path", "adminpass", "aMigrationName", []string{"onedb", "twodb"}, 1234, false, 0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ranCommands, gc.HasLen, 2)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--port", "1234", "--host", "localhost", "-u", "admin", "-p", "adminpass", "--db=onedb", "/fake/tmp/dir/migrateToaMigrationNamedump/onedb"})
	c.Assert(command.ranCommands[1], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--port", "1234", "--host", "localhost", "-u", "admin", "-p", "adminpass", "--db=twodb", "/fake/tmp/dir/migrateToaMigrationNamedump/twodb"})
}

func (s *UpgradeMongoSuite) TestMongoRestoreRetries(c *gc.C) {
	command := fakeRunCommand{}
	err := mongoRestoreCall(command.runCommandFail, "/fake/tmp/dir", "/fake/mongo/path", "", "aMigrationName", []string{}, 1234, false, 0)
	c.Assert(err, gc.ErrorMatches, "cannot restore dbs got: this failed: a generic error")
	c.Assert(command.ranCommands, gc.HasLen, 60)
	for i := range command.ranCommands {
		c.Log(fmt.Sprintf("Checking attempt %d", i))
		c.Assert(command.ranCommands[i], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--port", "1234", "--host", "localhost", "/fake/tmp/dir/migrateToaMigrationNamedump"})
	}
}

type fakeMgoSesion struct {
	ranClose int
}

func (f *fakeMgoSesion) Close() {
	f.ranClose++
}

type fakeMgoDb struct {
	ranAction string
}

func (f *fakeMgoDb) Run(action interface{}, res interface{}) error {
	f.ranAction = action.(string)
	resM := res.(*bson.M)
	(*resM)["ok"] = float64(1)
	return nil
}

func (f *fakeRunCommand) dialAndLogin(*mongo.MongoInfo) (mgoSession, mgoDb, error) {
	f.ranCommands = append(f.ranCommands, []string{"DialAndlogin"})
	return f.mgoSession, f.mgoDb, nil
}

func (f *fakeRunCommand) satisfyPrerequisites(string) error {
	f.ranCommands = append(f.ranCommands, []string{"SatisfyPrerequisites"})
	return nil
}

func (f *fakeRunCommand) createTempDir() (string, error) {
	f.ranCommands = append(f.ranCommands, []string{"CreateTempDir"})
	return "/fake/temp/dir", nil
}

func (f *fakeRunCommand) startService() error {
	f.ranCommands = append(f.ranCommands, []string{"mongo.StartService"})
	return nil
}
func (f *fakeRunCommand) stopService() error {
	f.ranCommands = append(f.ranCommands, []string{"mongo.StopService"})
	return nil
}
func (f *fakeRunCommand) reStartService() error {
	f.ranCommands = append(f.ranCommands, []string{"mongo.ReStartService"})
	return nil
}
func (f *fakeRunCommand) reStartServiceFail() error {
	f.ranCommands = append(f.ranCommands, []string{"mongo.ReStartServiceFail"})
	return errors.New("failing restart")
}
func (f *fakeRunCommand) ensureServiceInstalled(dataDir string, statePort, oplogSizeMB int, setNumaControlPolicy bool, version mongo.Version, auth bool) error {
	ran := []string{"mongo.EnsureServiceInstalled",
		dataDir,
		strconv.Itoa(statePort),
		strconv.Itoa(oplogSizeMB),
		strconv.FormatBool(setNumaControlPolicy),
		version.String(),
		strconv.FormatBool(auth)}

	f.ranCommands = append(f.ranCommands, ran)
	return nil
}
func (f *fakeRunCommand) mongoDialInfo(info mongo.Info, opts mongo.DialOpts) (*mgo.DialInfo, error) {
	ran := []string{"mongo.DialInfo"}
	f.ranCommands = append(f.ranCommands, ran)
	return &mgo.DialInfo{}, nil
}
func (f *fakeRunCommand) initiateMongoServer(args peergrouper.InitiateMongoParams, force bool) error {
	ran := []string{"peergrouper.InitiateMongoServer"}
	f.ranCommands = append(f.ranCommands, ran)
	return nil
}

func (f *fakeRunCommand) discoverService(serviceName string, c common.Conf) (service.Service, error) {
	ran := []string{"service.DiscoverService", serviceName}
	f.ranCommands = append(f.ranCommands, ran)
	return f.service, nil
}

func (f *fakeRunCommand) fsCopy(src, dst string) error {
	ran := []string{"fs.Copy", src, dst}
	f.ranCommands = append(f.ranCommands, ran)
	return nil
}

func (f *fakeRunCommand) replicaRemove(s mgoSession, addrs ...string) error {
	ran := []string{"replicaRemove"}
	f.ranCommands = append(f.ranCommands, ran)
	return nil
}

func (f *fakeRunCommand) replicaAdd(s mgoSession, members ...replicaset.Member) error {
	ran := []string{"replicaAdd"}
	f.ranCommands = append(f.ranCommands, ran)
	return nil
}

type fakeService struct {
	ranCommands []string
}

func (f *fakeService) Start() error {
	f.ranCommands = append(f.ranCommands, "Start")
	return nil
}

func (f *fakeService) Stop() error {
	f.ranCommands = append(f.ranCommands, "Stop")
	return nil
}

func (f *fakeService) Install() error {
	f.ranCommands = append(f.ranCommands, "Install")
	return nil
}

func (f *fakeService) Remove() error {
	f.ranCommands = append(f.ranCommands, "Remove")
	return nil
}

func (f *fakeService) Name() string {
	f.ranCommands = append(f.ranCommands, "Name")
	return "FakeService"
}

func (f *fakeService) Conf() common.Conf {
	f.ranCommands = append(f.ranCommands, "Conf")
	return common.Conf{}
}

func (f *fakeService) Running() (bool, error) {
	f.ranCommands = append(f.ranCommands, "Running")
	return true, nil
}

func (f *fakeService) Exists() (bool, error) {
	f.ranCommands = append(f.ranCommands, "Exists")
	return true, nil
}

func (f *fakeService) Installed() (bool, error) {
	f.ranCommands = append(f.ranCommands, "Installed")
	return true, nil
}

func (f *fakeService) InstallCommands() ([]string, error) {
	f.ranCommands = append(f.ranCommands, "InstalledCommands")
	return []string{"echo", "install"}, nil
}

func (f *fakeService) StartCommands() ([]string, error) {
	f.ranCommands = append(f.ranCommands, "StartCommands")
	return []string{"echo", "start"}, nil
}

func (s *UpgradeMongoCommandSuite) createFakeAgentConf(c *gc.C, agentDir string, mongoVersion mongo.Version) {
	attributeParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: agentDir,
		},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: version.Current,
		Password:          "sekrit",
		CACert:            "ca cert",
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
		Model:             testing.ModelTag,
	}

	servingInfo := params.StateServingInfo{
		Cert:           "cert",
		PrivateKey:     "key",
		CAPrivateKey:   "ca key",
		StatePort:      69,
		APIPort:        47,
		SharedSecret:   "shared",
		SystemIdentity: "identity",
	}
	conf, err := agent.NewStateMachineConfig(attributeParams, servingInfo)
	c.Check(err, jc.ErrorIsNil)
	conf.SetMongoVersion(mongoVersion)
	err = conf.Write()
	c.Check(err, jc.ErrorIsNil)
}

func (s *UpgradeMongoCommandSuite) TestRun(c *gc.C) {
	session := fakeMgoSesion{}
	db := fakeMgoDb{}
	service := fakeService{}
	command := fakeRunCommand{
		mgoSession: &session,
		mgoDb:      &db,
		service:    &service,
	}

	testDir := c.MkDir()
	testAgentConfig := agent.ConfigPath(testDir, names.NewMachineTag("0"))
	s.createFakeAgentConf(c, testDir, mongo.Mongo24)

	upgradeMongoCommand := &UpgradeMongoCommand{
		machineTag:     "0",
		series:         "vivid",
		configFilePath: testAgentConfig,
		tmpDir:         "/fake/temp/dir",

		stat:                 command.stat,
		remove:               command.remove,
		mkdir:                command.mkdir,
		runCommand:           command.runCommand,
		dialAndLogin:         command.dialAndLogin,
		satisfyPrerequisites: command.satisfyPrerequisites,
		createTempDir:        command.createTempDir,
		discoverService:      command.discoverService,
		fsCopy:               command.fsCopy,
		osGetenv:             command.getenv,

		mongoStart:                  command.startService,
		mongoStop:                   command.stopService,
		mongoRestart:                command.reStartService,
		mongoEnsureServiceInstalled: command.ensureServiceInstalled,
		mongoDialInfo:               command.mongoDialInfo,
		initiateMongoServer:         command.initiateMongoServer,
		replicasetAdd:               command.replicaAdd,
		replicasetRemove:            command.replicaRemove,
	}

	err := upgradeMongoCommand.run()
	c.Assert(err, jc.ErrorIsNil)

	expectedCommands := [][]string{
		[]string{"getenv", "UPSTART_JOB"},
		[]string{"service.DiscoverService", "bogus_daemon"},
		[]string{"CreateTempDir"},
		[]string{"SatisfyPrerequisites"},
		[]string{"CreateTempDir"},
		[]string{"mongo.StopService"},
		[]string{"stat", "/var/lib/juju/db"},
		[]string{"mkdir", "/fake/temp/dir/24"},
		[]string{"fs.Copy", "/var/lib/juju/db", "/fake/temp/dir/24/db"},
		[]string{"mongo.StartService"},
		[]string{"mongo.StopService"},
		[]string{"/usr/lib/juju/bin/mongod", "--dbpath", "/var/lib/juju/db", "--replSet", "juju", "--upgrade"},
		[]string{"mongo.EnsureServiceInstalled", testDir, "69", "0", "false", "2.6/mmapv1", "true"},
		[]string{"mongo.StartService"},
		[]string{"DialAndlogin"},
		[]string{"mongo.ReStartService"},
		[]string{"/usr/lib/juju/mongo2.6/bin/mongodump", "--ssl", "-u", "admin", "-p", "sekrit", "--port", "69", "--host", "localhost", "--out", "/fake/temp/dir/migrateTo30dump"},
		[]string{"mongo.StopService"},
		[]string{"mongo.EnsureServiceInstalled", testDir, "69", "0", "false", "3.0/mmapv1", "true"},
		[]string{"mongo.StartService"},
	}
	c.Assert(command.ranCommands, gc.DeepEquals, expectedCommands)
	c.Assert(session.ranClose, gc.Equals, 2)
	c.Assert(db.ranAction, gc.Equals, "authSchemaUpgrade")
	c.Assert(service.ranCommands, gc.DeepEquals, []string{"Stop", "Start"})
}

func (s *UpgradeMongoCommandSuite) TestRunRollback(c *gc.C) {
	session := fakeMgoSesion{}
	db := fakeMgoDb{}
	service := fakeService{}
	command := fakeRunCommand{
		mgoSession: &session,
		mgoDb:      &db,
		service:    &service,
	}

	tempDir := c.MkDir()
	testAgentConfig := agent.ConfigPath(tempDir, names.NewMachineTag("0"))
	s.createFakeAgentConf(c, tempDir, mongo.Mongo24)

	upgradeMongoCommand := &UpgradeMongoCommand{
		machineTag:     "0",
		series:         "vivid",
		configFilePath: testAgentConfig,
		tmpDir:         "/fake/temp/dir",

		stat:                 command.stat,
		remove:               command.remove,
		mkdir:                command.mkdir,
		runCommand:           command.runCommand,
		dialAndLogin:         command.dialAndLogin,
		satisfyPrerequisites: command.satisfyPrerequisites,
		createTempDir:        command.createTempDir,
		discoverService:      command.discoverService,
		fsCopy:               command.fsCopy,
		osGetenv:             command.getenv,

		mongoStart:                  command.startService,
		mongoStop:                   command.stopService,
		mongoRestart:                command.reStartServiceFail,
		mongoEnsureServiceInstalled: command.ensureServiceInstalled,
		mongoDialInfo:               command.mongoDialInfo,
		initiateMongoServer:         command.initiateMongoServer,
		replicasetAdd:               command.replicaAdd,
		replicasetRemove:            command.replicaRemove,
	}

	err := upgradeMongoCommand.run()
	// It is nil because making Stop fail would be a less useful test.
	c.Assert(err, gc.ErrorMatches, "failed upgrade and juju start after rollbacking upgrade: <nil>: cannot upgrade from mongo 2.4 to 2.6: cannot restart mongodb 2.6 service: failing restart")

	expectedCommands := [][]string{
		[]string{"getenv", "UPSTART_JOB"},
		[]string{"service.DiscoverService", "bogus_daemon"},
		[]string{"CreateTempDir"},
		[]string{"SatisfyPrerequisites"},
		[]string{"CreateTempDir"},
		[]string{"mongo.StopService"},
		[]string{"stat", "/var/lib/juju/db"},
		[]string{"mkdir", "/fake/temp/dir/24"},
		[]string{"fs.Copy", "/var/lib/juju/db", "/fake/temp/dir/24/db"},
		[]string{"mongo.StartService"},
		[]string{"mongo.StopService"},
		[]string{"/usr/lib/juju/bin/mongod", "--dbpath", "/var/lib/juju/db", "--replSet", "juju", "--upgrade"},
		[]string{"mongo.EnsureServiceInstalled", tempDir, "69", "0", "false", "2.6/mmapv1", "true"},
		[]string{"mongo.StartService"},
		[]string{"DialAndlogin"},
		[]string{"mongo.ReStartServiceFail"},
		[]string{"mongo.StopService"},
		[]string{"remove", "/var/lib/juju/db"},
		[]string{"mongo.StartService"},
	}

	c.Assert(command.ranCommands, gc.DeepEquals, expectedCommands)
	c.Assert(session.ranClose, gc.Equals, 2)
	c.Assert(db.ranAction, gc.Equals, "authSchemaUpgrade")
	c.Assert(service.ranCommands, gc.DeepEquals, []string{"Stop", "Start"})
}

func (s *UpgradeMongoCommandSuite) TestRunContinuesWhereLeft(c *gc.C) {
	session := fakeMgoSesion{}
	db := fakeMgoDb{}
	service := fakeService{}

	command := fakeRunCommand{
		mgoSession: &session,
		mgoDb:      &db,
		service:    &service,
	}

	testDir := c.MkDir()
	testAgentConfig := agent.ConfigPath(testDir, names.NewMachineTag("0"))
	s.createFakeAgentConf(c, testDir, mongo.Mongo26)

	upgradeMongoCommand := &UpgradeMongoCommand{
		machineTag:     "0",
		series:         "vivid",
		configFilePath: testAgentConfig,
		tmpDir:         "/fake/temp/dir",

		stat:                 command.stat,
		remove:               command.remove,
		mkdir:                command.mkdir,
		runCommand:           command.runCommand,
		dialAndLogin:         command.dialAndLogin,
		satisfyPrerequisites: command.satisfyPrerequisites,
		createTempDir:        command.createTempDir,
		discoverService:      command.discoverService,
		fsCopy:               command.fsCopy,
		osGetenv:             command.getenv,

		mongoStart:                  command.startService,
		mongoStop:                   command.stopService,
		mongoRestart:                command.reStartService,
		mongoEnsureServiceInstalled: command.ensureServiceInstalled,
		mongoDialInfo:               command.mongoDialInfo,
		initiateMongoServer:         command.initiateMongoServer,
		replicasetAdd:               command.replicaAdd,
		replicasetRemove:            command.replicaRemove,
	}

	err := upgradeMongoCommand.run()
	c.Assert(err, jc.ErrorIsNil)
	expectedCommands := [][]string{
		[]string{"getenv", "UPSTART_JOB"},
		[]string{"service.DiscoverService", "bogus_daemon"},
		[]string{"CreateTempDir"},
		[]string{"SatisfyPrerequisites"},
		[]string{"/usr/lib/juju/mongo2.6/bin/mongodump", "--ssl", "-u", "admin", "-p", "sekrit", "--port", "69", "--host", "localhost", "--out", "/fake/temp/dir/migrateTo30dump"},
		[]string{"mongo.StopService"},
		[]string{"mongo.EnsureServiceInstalled", testDir, "69", "0", "false", "3.0/mmapv1", "true"},
		[]string{"mongo.StartService"},
	}
	c.Assert(command.ranCommands, gc.DeepEquals, expectedCommands)
}
