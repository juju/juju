// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/names"
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
	f.ranCommands = append(f.ranCommands, []string{statFile})
	return fakeFileInfo{}, nil
}

func (f *fakeRunCommand) rename(from, to string) error {
	f.ranCommands = append(f.ranCommands, []string{from, to})
	return nil
}

func (f *fakeRunCommand) mkdir(dirname string, mode os.FileMode) error {
	f.ranCommands = append(f.ranCommands, []string{dirname})
	return nil
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

func (s *UpgradeMongoSuite) TestRenameOldDb(c *gc.C) {
	command := fakeRunCommand{}
	err := renameOldDbCall("/a/fake/datadir", command.stat, command.rename, command.mkdir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ranCommands, gc.HasLen, 3)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/a/fake/datadir/db"})
	c.Assert(command.ranCommands[1], gc.DeepEquals, []string{"/a/fake/datadir/db", "/a/fake/datadir/db.old"})
	c.Assert(command.ranCommands[2], gc.DeepEquals, []string{"/a/fake/datadir/db"})
}

func (s *UpgradeMongoSuite) TestMongoDump(c *gc.C) {
	command := fakeRunCommand{}
	out, err := mongoDumpCall(command.runCommand, "/fake/mongo/path", "adminpass", "aMigrationName", 1234)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, "")
	c.Assert(command.ranCommands, gc.HasLen, 1)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/fake/mongo/path/mongodump", "--ssl", "-u", "admin", "-p", "adminpass", "--port", "1234", "--host", "localhost", "--out", "/home/ubuntu/migrateToaMigrationNamedump"})
}

func (s *UpgradeMongoSuite) TestMongoDumpRetries(c *gc.C) {
	command := fakeRunCommand{}
	out, err := mongoDumpCall(command.runCommandFail, "/fake/mongo/path", "", "aMigrationName", 1234)
	c.Assert(err, gc.ErrorMatches, "cannot dump mongo db: a generic error")
	c.Assert(out, gc.Equals, "this failed")
	c.Assert(command.ranCommands, gc.HasLen, 60)
	for i := range command.ranCommands {
		c.Log("Checking attempt %d", i)
		c.Assert(command.ranCommands[i], gc.DeepEquals, []string{"/fake/mongo/path/mongodump", "--ssl", "-u", "admin", "-p", "", "--port", "1234", "--host", "localhost", "--out", "/home/ubuntu/migrateToaMigrationNamedump"})
	}
}

func (s *UpgradeMongoSuite) TestMongoRestore(c *gc.C) {
	command := fakeRunCommand{}
	err := mongoRestoreCall(command.runCommand, "/fake/mongo/path", "adminpass", "aMigrationName", []string{}, 1234)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ranCommands, gc.HasLen, 1)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--sslAllowInvalidCertificates", "--port", "1234", "--host", "localhost", "-u", "admin", "-p", "adminpass", "/home/ubuntu/migrateToaMigrationNamedump"})
}

func (s *UpgradeMongoSuite) TestMongoRestoreWithoutAdmin(c *gc.C) {
	command := fakeRunCommand{}
	err := mongoRestoreCall(command.runCommand, "/fake/mongo/path", "", "aMigrationName", []string{}, 1234)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ranCommands, gc.HasLen, 1)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--sslAllowInvalidCertificates", "--port", "1234", "--host", "localhost", "/home/ubuntu/migrateToaMigrationNamedump"})
}

func (s *UpgradeMongoSuite) TestMongoRestoreWithDBs(c *gc.C) {
	command := fakeRunCommand{}
	err := mongoRestoreCall(command.runCommand, "/fake/mongo/path", "adminpass", "aMigrationName", []string{"onedb", "twodb"}, 1234)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ranCommands, gc.HasLen, 2)
	c.Assert(command.ranCommands[0], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--sslAllowInvalidCertificates", "--port", "1234", "--host", "localhost", "-u", "admin", "-p", "adminpass", "--db=onedb", "/home/ubuntu/migrateToaMigrationNamedump/onedb"})
	c.Assert(command.ranCommands[1], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--sslAllowInvalidCertificates", "--port", "1234", "--host", "localhost", "-u", "admin", "-p", "adminpass", "--db=twodb", "/home/ubuntu/migrateToaMigrationNamedump/twodb"})
}

func (s *UpgradeMongoSuite) TestMongoRestoreRetries(c *gc.C) {
	command := fakeRunCommand{}
	err := mongoRestoreCall(command.runCommandFail, "/fake/mongo/path", "", "aMigrationName", []string{}, 1234)
	c.Assert(err, gc.ErrorMatches, "cannot restore dbs got: this failed: a generic error")
	c.Assert(command.ranCommands, gc.HasLen, 60)
	for i := range command.ranCommands {
		c.Log("Checking attempt %d", i)
		c.Assert(command.ranCommands[i], gc.DeepEquals, []string{"/fake/mongo/path/mongorestore", "--ssl", "--sslAllowInvalidCertificates", "--port", "1234", "--host", "localhost", "/home/ubuntu/migrateToaMigrationNamedump"})
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

func (f *fakeRunCommand) startService(string) error {
	f.ranCommands = append(f.ranCommands, []string{"mongo.StartService"})
	return nil
}
func (f *fakeRunCommand) stopService(string) error {
	f.ranCommands = append(f.ranCommands, []string{"mongo.StopService"})
	return nil
}
func (f *fakeRunCommand) reStartService(string) error {
	f.ranCommands = append(f.ranCommands, []string{"mongo.ReStartService"})
	return nil
}
func (f *fakeRunCommand) ensureServiceInstalled(dataDir, namespace string, statePort, oplogSizeMB int, setNumaControlPolicy bool, version mongo.Version, auth bool) error {
	ran := []string{"mongo.EnsureServiceInstalled",
		dataDir,
		namespace,
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

func (s *UpgradeMongoCommandSuite) createFakeAgentConf(c *gc.C, agentDir string) {
	attributeParams := agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: agentDir,
		},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: version.Current.Number,
		Password:          "sekrit",
		CACert:            "ca cert",
		StateAddresses:    []string{"localhost:1234"},
		APIAddresses:      []string{"localhost:1235"},
		Nonce:             "a nonce",
		Environment:       testing.EnvironmentTag,
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
	err = conf.Write()
	c.Check(err, jc.ErrorIsNil)
}

func (s *UpgradeMongoCommandSuite) TestRun(c *gc.C) {
	session := fakeMgoSesion{}
	db := fakeMgoDb{}
	command := fakeRunCommand{
		mgoSession: &session,
		mgoDb:      &db,
	}

	testDir := c.MkDir()
	testAgentConfig := agent.ConfigPath(testDir, names.NewMachineTag("0"))
	s.createFakeAgentConf(c, testDir)

	upgradeMongoCommand := &UpgradeMongoCommand{
		machineTag:     "0",
		series:         "vivid",
		namespace:      "",
		configFilePath: testAgentConfig,

		stat:                 command.stat,
		rename:               command.rename,
		mkdir:                command.mkdir,
		runCommand:           command.runCommand,
		dialAndLogin:         command.dialAndLogin,
		satisfyPrerequisites: command.satisfyPrerequisites,

		mongoStart:                  command.startService,
		mongoStop:                   command.stopService,
		mongoRestart:                command.reStartService,
		mongoEnsureServiceInstalled: command.ensureServiceInstalled,
		mongoDialInfo:               command.mongoDialInfo,
		initiateMongoServer:         command.initiateMongoServer,
	}

	err := upgradeMongoCommand.run()
	c.Assert(err, jc.ErrorIsNil)
	expectedCommands := [][]string{
		[]string{"SatisfyPrerequisites"},
		[]string{"/usr/lib/juju/bin/mongodump", "--ssl", "-u", "admin", "-p", "sekrit", "--port", "69", "--host", "localhost", "--out", "/home/ubuntu/migrateTo26dump"},
		[]string{"mongo.StopService"},
		[]string{"/usr/lib/juju/bin/mongod", "--dbpath", "/var/lib/juju/db", "--replSet", "juju", "--upgrade"},
		[]string{"mongo.EnsureServiceInstalled", "/tmp/check-5577006791947779410/0", "", "69", "0", "false", "2.6/mmapiv2", "true"},
		[]string{"mongo.StartService"},
		[]string{"DialAndlogin"},
		[]string{"mongo.ReStartService"},
		[]string{"/usr/lib/juju/mongo2.6/bin/mongodump", "--ssl", "-u", "admin", "-p", "sekrit", "--port", "69", "--host", "localhost", "--out", "/home/ubuntu/migrateTo30dump"},
		[]string{"mongo.StopService"},
		[]string{"mongo.EnsureServiceInstalled", "/tmp/check-5577006791947779410/0", "", "69", "0", "false", "3.0/mmapiv2", "true"},
		[]string{"mongo.StartService"},
		[]string{"/usr/lib/juju/mongo3/bin/mongodump", "--ssl", "-u", "admin", "-p", "sekrit", "--port", "69", "--host", "localhost", "--out", "/home/ubuntu/migrateToTigerdump"},
		[]string{"mongo.StopService"},
		[]string{"/tmp/check-5577006791947779410/0/db"},
		[]string{"/tmp/check-5577006791947779410/0/db", "/tmp/check-5577006791947779410/0/db.old"},
		[]string{"/tmp/check-5577006791947779410/0/db"},
		[]string{"mongo.EnsureServiceInstalled", "/tmp/check-5577006791947779410/0", "", "69", "0", "false", "3.0/wiredTiger", "false"},
		[]string{"mongo.DialInfo"},
		[]string{"mongo.StartService"},
		[]string{"peergrouper.InitiateMongoServer"},
		[]string{"/usr/lib/juju/mongo3/bin/mongorestore", "--ssl", "--sslAllowInvalidCertificates", "--port", "69", "--host", "localhost", "--db=juju", "/home/ubuntu/migrateToTigerdump/juju"},
		[]string{"/usr/lib/juju/mongo3/bin/mongorestore", "--ssl", "--sslAllowInvalidCertificates", "--port", "69", "--host", "localhost", "--db=admin", "/home/ubuntu/migrateToTigerdump/admin"},
		[]string{"/usr/lib/juju/mongo3/bin/mongorestore", "--ssl", "--sslAllowInvalidCertificates", "--port", "69", "--host", "localhost", "--db=logs", "/home/ubuntu/migrateToTigerdump/logs"},
		[]string{"/usr/lib/juju/mongo3/bin/mongorestore", "--ssl", "--sslAllowInvalidCertificates", "--port", "69", "--host", "localhost", "--db=presence", "/home/ubuntu/migrateToTigerdump/presence"},
		[]string{"mongo.EnsureServiceInstalled", "/tmp/check-5577006791947779410/0", "", "69", "0", "false", "3.0/wiredTiger", "true"},
		[]string{"mongo.ReStartService"}}
	c.Assert(command.ranCommands, gc.DeepEquals, expectedCommands)
}
