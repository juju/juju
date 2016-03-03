// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/names"
	"github.com/juju/replicaset"
	"github.com/juju/utils"
	"github.com/juju/utils/fs"
	"github.com/juju/utils/packaging/manager"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gnuflag"
)

const KeyUpgradeBackup = "mongo-upgrade-backup"

func createTempDir() (string, error) {
	return ioutil.TempDir("", "")
}

// NewUpgradeMongoCommand returns a new UpgradeMongo command initialized with
// the default helper functions.
func NewUpgradeMongoCommand() *UpgradeMongoCommand {
	return &UpgradeMongoCommand{
		stat:                 os.Stat,
		remove:               os.RemoveAll,
		mkdir:                os.Mkdir,
		runCommand:           utils.RunCommand,
		dialAndLogin:         dialAndLogin,
		satisfyPrerequisites: satisfyPrerequisites,
		createTempDir:        createTempDir,
		discoverService:      service.DiscoverService,
		fsCopy:               fs.Copy,
		osGetenv:             os.Getenv,

		mongoStart:                  mongo.StartService,
		mongoStop:                   mongo.StopService,
		mongoRestart:                mongo.ReStartService,
		mongoEnsureServiceInstalled: mongo.EnsureServiceInstalled,
		mongoDialInfo:               mongo.DialInfo,
		initiateMongoServer:         peergrouper.InitiateMongoServer,
		replicasetAdd:               replicaAddCall,
		replicasetRemove:            replicaRemoveCall,
	}
}

type statFunc func(string) (os.FileInfo, error)
type removeFunc func(string) error
type mkdirFunc func(string, os.FileMode) error
type createTempDirFunc func() (string, error)
type discoverService func(string, common.Conf) (service.Service, error)
type fsCopyFunc func(string, string) error
type osGetenv func(string) string

type utilsRun func(command string, args ...string) (output string, err error)

type mgoSession interface {
	Close()
}

type mgoDb interface {
	Run(interface{}, interface{}) error
}

type dialAndLogger func(*mongo.MongoInfo) (mgoSession, mgoDb, error)

type requisitesSatisfier func(string) error

type mongoService func() error
type mongoEnsureService func(string, int, int, bool, mongo.Version, bool) error
type mongoDialInfo func(mongo.Info, mongo.DialOpts) (*mgo.DialInfo, error)

type initiateMongoServerFunc func(peergrouper.InitiateMongoParams, bool) error

type replicaAddFunc func(mgoSession, ...replicaset.Member) error
type replicaRemoveFunc func(mgoSession, ...string) error

// UpgradeMongoCommand represents a jujud upgrade-mongo command.
type UpgradeMongoCommand struct {
	cmd.CommandBase
	machineTag     string
	series         string
	configFilePath string
	agentConfig    agent.ConfigSetterWriter
	tmpDir         string
	backupPath     string
	rollback       bool
	slave          bool
	wiredTiger     bool
	members        string

	// utils used by this struct.
	stat                 statFunc
	remove               removeFunc
	mkdir                mkdirFunc
	runCommand           utilsRun
	dialAndLogin         dialAndLogger
	satisfyPrerequisites requisitesSatisfier
	createTempDir        createTempDirFunc
	discoverService      discoverService
	fsCopy               fsCopyFunc
	osGetenv             osGetenv

	// mongo related utils.
	mongoStart                  mongoService
	mongoStop                   mongoService
	mongoRestart                mongoService
	mongoEnsureServiceInstalled mongoEnsureService
	mongoDialInfo               mongoDialInfo
	initiateMongoServer         initiateMongoServerFunc
	replicasetAdd               replicaAddFunc
	replicasetRemove            replicaRemoveFunc
}

// Info returns a decription of the command.
func (*UpgradeMongoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-mongo",
		Purpose: "upgrade state server to mongo 3",
	}
}

// SetFlags adds the flags for this command to the passed gnuflag.FlagSet.
func (u *UpgradeMongoCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&u.machineTag, "machinetag", "machine-0", "unique tag identifier for machine to be upgraded")
	f.StringVar(&u.series, "series", "", "series for the machine")
	f.StringVar(&u.configFilePath, "configfile", "", "path to the config file")
	f.StringVar(&u.members, "members", "", "a comma separated list of replicaset member ips")
	f.BoolVar(&u.rollback, "rollback", false, "rollback a previous attempt at upgrading that was cut in the process")
	f.BoolVar(&u.slave, "slave", false, "this is a slave machine in a replicaset")
	f.BoolVar(&u.wiredTiger, "wiredTiger", false, "use wired tiger storage, requires a mongo 3 with js enabled")
}

// Init initializes the command for running.
func (u *UpgradeMongoCommand) Init(args []string) error {
	return nil
}

// Run migrates an environment to mongo 3.
func (u *UpgradeMongoCommand) Run(ctx *cmd.Context) error {
	return u.run()
}

func (u *UpgradeMongoCommand) run() (err error) {
	dataDir, err := paths.DataDir(u.series)
	if err != nil {
		return errors.Annotatef(err, "cannot determine data dir for %q", u.series)
	}
	if u.configFilePath == "" {
		machineTag, err := names.ParseMachineTag(u.machineTag)
		if err != nil {
			return errors.Annotatef(err, "%q is not a valid machine tag", u.machineTag)
		}
		u.configFilePath = agent.ConfigPath(dataDir, machineTag)
	}
	u.agentConfig, err = agent.ReadConfig(u.configFilePath)
	if err != nil {
		return errors.Annotatef(err, "cannot read config file in %q", u.configFilePath)
	}

	current := u.agentConfig.MongoVersion()

	agentServiceName := u.agentConfig.Value(agent.AgentServiceName)
	if agentServiceName == "" {
		// For backwards compatibility, handle lack of AgentServiceName.
		agentServiceName = u.osGetenv("UPSTART_JOB")
	}
	if agentServiceName == "" {
		return errors.New("cannot determine juju service name")
	}
	svc, err := u.discoverService(agentServiceName, common.Conf{})
	if err != nil {
		return errors.Annotate(err, "cannot determine juju service")
	}
	if err := svc.Stop(); err != nil {
		return errors.Annotate(err, "cannot stop juju to begin migration")
	}
	defer func() {
		svcErr := svc.Start()
		if err != nil {
			err = errors.Annotatef(err, "failed upgrade and juju start after rollbacking upgrade: %v", svcErr)
		} else {
			err = errors.Annotate(svcErr, "could not start juju after upgrade")
		}
	}()
	if !u.slave {
		defer u.replicaAdd()
	}
	if u.rollback {
		origin := u.agentConfig.Value(KeyUpgradeBackup)
		if origin == "" {
			return errors.New("no available backup")
		}
		return u.rollbackCopyBackup(dataDir, origin)
	}

	u.tmpDir, err = u.createTempDir()
	if err != nil {
		return errors.Annotate(err, "could not create a temporary directory for the migration")
	}

	logger.Infof("begin migration to mongo 3")

	if err := u.satisfyPrerequisites(u.series); err != nil {
		return errors.Annotate(err, "cannot satisfy pre-requisites for the migration")
	}
	if current == mongo.Mongo24 || current == mongo.MongoUpgrade {
		if u.slave {
			return u.upgradeSlave(dataDir)
		}
		u.replicaRemove()
		if err := u.maybeUpgrade24to26(dataDir); err != nil {
			defer func() {
				if u.backupPath == "" {
					return
				}
				logger.Infof("will roll back after failed 2.6 upgrade")
				if err := u.rollbackCopyBackup(dataDir, u.backupPath); err != nil {
					logger.Errorf("could not rollback the upgrade: %v", err)
				}
			}()
			return errors.Annotate(err, "cannot upgrade from mongo 2.4 to 2.6")
		}
		current = mongo.Mongo26
	}
	if current == mongo.Mongo26 || current == mongo.Mongo30 {
		if err := u.maybeUpgrade26to31(dataDir); err != nil {
			defer func() {
				if u.backupPath == "" {
					return
				}
				logger.Infof("will roll back after failed 3.0 upgrade")
				if err := u.rollbackCopyBackup(dataDir, u.backupPath); err != nil {
					logger.Errorf("could not rollback the upgrade: %v", err)
				}
			}()
			return errors.Annotate(err, "cannot upgrade from mongo 2.6 to 3")
		}
	}
	return nil
}

func replicaRemoveCall(session mgoSession, addrs ...string) error {
	mSession := session.(*mgo.Session)
	if err := replicaset.Remove(mSession, addrs...); err != nil {
		return errors.Annotate(err, "cannot resume HA")
	}
	return nil
}

func (u *UpgradeMongoCommand) replicaRemove() error {
	if u.members == "" {
		return nil
	}
	info, ok := u.agentConfig.MongoInfo()
	if !ok {
		return errors.New("cannot get mongo info from agent config to resume HA")
	}

	var session mgoSession
	var err error
	for attempt := connectAfterRestartRetryAttempts.Start(); attempt.Next(); {
		// Try to connect, retry a few times until the db comes up.
		session, _, err = u.dialAndLogin(info)
		if err == nil {
			break
		}
		logger.Errorf("cannot open mongo connection to resume HA auth schema: %v", err)
	}
	if err != nil {
		return errors.Annotate(err, "error dialing mongo to resume HA")
	}
	defer session.Close()
	addrs := strings.Split(u.members, ",")

	if err := u.replicasetRemove(session, addrs...); err != nil {
		return errors.Annotate(err, "cannot resume HA")
	}
	return nil
}

func replicaAddCall(session mgoSession, members ...replicaset.Member) error {
	mSession := session.(*mgo.Session)
	if err := replicaset.Add(mSession, members...); err != nil {
		return errors.Annotate(err, "cannot resume HA")
	}
	return nil
}

func (u *UpgradeMongoCommand) replicaAdd() error {
	if u.members == "" {
		return nil
	}
	info, ok := u.agentConfig.MongoInfo()
	if !ok {
		return errors.New("cannot get mongo info from agent config to resume HA")
	}

	var session mgoSession
	var err error
	for attempt := connectAfterRestartRetryAttempts.Start(); attempt.Next(); {
		// Try to connect, retry a few times until the db comes up.
		session, _, err = u.dialAndLogin(info)
		if err == nil {
			break
		}
		logger.Errorf("cannot open mongo connection to resume HA auth schema: %v", err)
	}
	if err != nil {
		return errors.Annotate(err, "error dialing mongo to resume HA")
	}
	defer session.Close()
	addrs := strings.Split(u.members, ",")
	members := make([]replicaset.Member, len(addrs))
	for i, addr := range addrs {
		members[i] = replicaset.Member{Address: addr}
	}

	if err := u.replicasetAdd(session, members...); err != nil {
		return errors.Annotate(err, "cannot resume HA")
	}
	return nil
}

// UpdateService will re-write the service scripts for mongo and restart it.
func (u *UpgradeMongoCommand) UpdateService(auth bool) error {
	var oplogSize int
	if oplogSizeString := u.agentConfig.Value(agent.MongoOplogSize); oplogSizeString != "" {
		var err error
		if oplogSize, err = strconv.Atoi(oplogSizeString); err != nil {
			return errors.Annotatef(err, "invalid oplog size: %q", oplogSizeString)
		}
	}

	var numaCtlPolicy bool
	if numaCtlString := u.agentConfig.Value(agent.NumaCtlPreference); numaCtlString != "" {
		var err error
		if numaCtlPolicy, err = strconv.ParseBool(numaCtlString); err != nil {
			return errors.Annotatef(err, "invalid numactl preference: %q", numaCtlString)
		}
	}
	ssi, _ := u.agentConfig.StateServingInfo()

	err := u.mongoEnsureServiceInstalled(u.agentConfig.DataDir(),
		ssi.StatePort,
		oplogSize,
		numaCtlPolicy,
		u.agentConfig.MongoVersion(),
		auth)
	return errors.Annotate(err, "cannot ensure mongodb service script is properly installed")
}

func (u *UpgradeMongoCommand) maybeUpgrade24to26(dataDir string) error {
	logger.Infof("backing up 2.4 MongoDB")
	var err error
	u.backupPath, err = u.copyBackupMongo("24", dataDir)
	if err != nil {
		return errors.Annotate(err, "could not do pre migration backup")
	}

	logger.Infof("stopping 2.4 MongoDB")
	if err := u.mongoStop(); err != nil {
		return errors.Annotate(err, "cannot stop mongo to perform 2.6 upgrade step")
	}

	// Run the not-so-optional --upgrade step on mongodb 2.6.
	if err := u.mongo26UpgradeStep(dataDir); err != nil {
		return errors.Annotate(err, "cannot run mongo 2.6 with --upgrade")
	}

	u.agentConfig.SetMongoVersion(mongo.Mongo26)
	if err := u.agentConfig.Write(); err != nil {
		return errors.Annotate(err, "could not update mongo version in agent.config")
	}

	if err := u.UpdateService(true); err != nil {
		return errors.Annotate(err, "cannot update mongo service to use mongo 2.6")
	}

	logger.Infof("starting 2.6 MongoDB")
	if err := u.mongoStart(); err != nil {
		return errors.Annotate(err, "cannot start mongo 2.6 to upgrade auth schema")
	}

	info, ok := u.agentConfig.MongoInfo()
	if !ok {
		return errors.New("cannot get mongo info from agent config")
	}

	var session mgoSession
	var db mgoDb
	for attempt := connectAfterRestartRetryAttempts.Start(); attempt.Next(); {
		// Try to connect, retry a few times until the db comes up.
		session, db, err = u.dialAndLogin(info)
		if err == nil {
			break
		}
		logger.Errorf("cannot open mongo connection to upgrade auth schema: %v", err)
	}
	if err != nil {
		return errors.Annotate(err, "error dialing mongo to upgrade auth schema")
	}
	defer session.Close()

	var res bson.M
	res = make(bson.M)
	err = db.Run("authSchemaUpgrade", &res)
	if err != nil {
		return errors.Annotate(err, "cannot upgrade auth schema")
	}

	if res["ok"].(float64) != 1 {
		return errors.Errorf("cannot upgrade auth schema :%s", res["message"])
	}
	session.Close()
	if err := u.mongoRestart(); err != nil {
		return errors.Annotate(err, "cannot restart mongodb 2.6 service")
	}
	return nil
}

func (u *UpgradeMongoCommand) maybeUpgrade26to31(dataDir string) error {
	jujuMongoPath := path.Dir(mongo.JujuMongod26Path)
	password := u.agentConfig.OldPassword()
	ssi, _ := u.agentConfig.StateServingInfo()
	port := ssi.StatePort
	current := u.agentConfig.MongoVersion()

	logger.Infof("backing up 2.6 MongoDB")
	if current == mongo.Mongo26 {
		// TODO(perrito666) dont ignore out if debug-log was used.
		_, err := u.mongoDump(jujuMongoPath, password, "30", port)
		if err != nil {
			return errors.Annotate(err, "could not do pre migration backup")
		}
		logger.Infof("pre 3.x migration dump ready.")
		if err := u.mongoStop(); err != nil {
			return errors.Annotate(err, "cannot stop mongo to update to mongo 3")
		}
		logger.Infof("mongo stopped")

		// Mongo 3, no wired tiger
		u.agentConfig.SetMongoVersion(mongo.Mongo30)
		if err := u.agentConfig.Write(); err != nil {
			return errors.Annotate(err, "could not update mongo version in agent.config")
		}
		logger.Infof(fmt.Sprintf("new mongo version set-up to %q", mongo.Mongo30.String()))

		if err := u.UpdateService(true); err != nil {
			return errors.Annotate(err, "cannot update service script")
		}
		logger.Infof("service startup scripts up to date")

		if err := u.mongoStart(); err != nil {
			return errors.Annotate(err, "cannot start mongo 3 to do a pre-tiger migration dump")
		}
		logger.Infof("started mongo")
		current = mongo.Mongo30
	}

	if current == mongo.Mongo30 && u.wiredTiger {
		jujuMongoPath = path.Dir(mongo.JujuMongod30Path)
		_, err := u.mongoDump(jujuMongoPath, password, "Tiger", port)
		if err != nil {
			return errors.Annotate(err, "could not do the tiger migration export")
		}
		logger.Infof("dumped to change storage")

		if err := u.mongoStop(); err != nil {
			return errors.Annotate(err, "cannot stop mongo to update to wired tiger")
		}
		logger.Infof("mongo stopped before storage migration")
		if err := u.removeOldDb(u.agentConfig.DataDir()); err != nil {
			return errors.Annotate(err, "cannot prepare the new db location for wired tiger")
		}
		logger.Infof("old db files removed")

		// Mongo 3, with wired tiger
		u.agentConfig.SetMongoVersion(mongo.Mongo30wt)
		if err := u.agentConfig.Write(); err != nil {
			return errors.Annotate(err, "could not update mongo version in agent.config")
		}
		logger.Infof("wired tiger set in agent.config")

		if err := u.UpdateService(false); err != nil {
			return errors.Annotate(err, "cannot update service script to use wired tiger")
		}
		logger.Infof("service startup script up to date")

		info, ok := u.agentConfig.MongoInfo()
		if !ok {
			return errors.New("cannot get mongo info from agent config")
		}

		logger.Infof("will create dialinfo for new mongo")
		//TODO(perrito666) make this into its own function
		dialOpts := mongo.DialOpts{}
		dialInfo, err := u.mongoDialInfo(info.Info, dialOpts)
		if err != nil {
			return errors.Annotate(err, "cannot obtain dial info")
		}

		if err := u.mongoStart(); err != nil {
			return errors.Annotate(err, "cannot start mongo 3 to restart replicaset")
		}
		logger.Infof("mongo started")

		// perhaps statehost port?
		// we need localhost, since there is no admin user
		peerHostPort := net.JoinHostPort("localhost", fmt.Sprint(ssi.StatePort))
		err = u.initiateMongoServer(peergrouper.InitiateMongoParams{
			DialInfo:       dialInfo,
			MemberHostPort: peerHostPort,
		}, true)
		if err != nil {
			return errors.Annotate(err, "cannot initiate replicaset")
		}
		logger.Infof("mongo initiated")

		// blobstorage might fail to restore in certain versions of
		// mongorestore because of a bug in mongorestore
		// that breaks gridfs restoration https://jira.mongodb.org/browse/TOOLS-939
		err = u.mongoRestore(jujuMongoPath, "", "Tiger", nil, port, true, 100)
		if err != nil {
			return errors.Annotate(err, "cannot restore the db.")
		}
		logger.Infof("mongo restored into the new storage")

		if err := u.UpdateService(true); err != nil {
			return errors.Annotate(err, "cannot update service script post wired tiger migration")
		}
		logger.Infof("service scripts up to date")

		if err := u.mongoRestart(); err != nil {
			return errors.Annotate(err, "cannot restart mongo service after upgrade")
		}
		logger.Infof("mongo restarted")
	}
	return nil
}

// connectAfterRestartRetryAttempts defines how many retries
// will there be when trying to connect to mongo after starting it.
var connectAfterRestartRetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Minute,
	Delay: 1 * time.Second,
}

// dialAndLogin returns a mongo session logged in as a user with administrative
// privileges
func dialAndLogin(mongoInfo *mongo.MongoInfo) (mgoSession, mgoDb, error) {
	var session *mgo.Session
	var err error
	opts := mongo.DefaultDialOpts()
	for attempt := connectAfterRestartRetryAttempts.Start(); attempt.Next(); {
		// Try to connect, retry a few times until the db comes up.
		session, err = mongo.DialWithInfo(mongoInfo.Info, opts)
		if err == nil {
			break
		}
		logger.Errorf("cannot open mongo connection to upgrade auth schema: %v", err)
	}
	if err != nil {
		return nil, nil, errors.Annotate(err, "timed out trying to dial mongo")
	}
	admin := session.DB("admin")
	if mongoInfo.Tag != nil {
		if err := admin.Login(mongoInfo.Tag.String(), mongoInfo.Password); err != nil {
			return nil, nil, errors.Annotatef(err, "cannot log in to admin database as %q", mongoInfo.Tag)
		}
	} else if mongoInfo.Password != "" {
		if err := admin.Login(mongo.AdminUser, mongoInfo.Password); err != nil {
			return nil, nil, errors.Annotate(err, "cannot log in to admin database")
		}
	}
	return session, admin, nil
}

func mongo26UpgradeStepCall(runCommand utilsRun, dataDir string) error {
	updateArgs := []string{"--dbpath", mongo.DbDir(dataDir), "--replSet", "juju", "--upgrade"}
	out, err := runCommand(mongo.JujuMongodPath, updateArgs...)
	logger.Infof(out)
	if err != nil {
		return errors.Annotate(err, "cannot upgrade mongo 2.4 data")
	}
	return nil
}

func (u *UpgradeMongoCommand) mongo26UpgradeStep(dataDir string) error {
	return mongo26UpgradeStepCall(u.runCommand, dataDir)
}

func removeOldDbCall(dataDir string, stat statFunc, remove removeFunc, mkdir mkdirFunc) error {
	dbPath := path.Join(dataDir, "db")

	fi, err := stat(dbPath)
	if err != nil {
		return errors.Annotatef(err, "cannot stat %q", dbPath)
	}

	if err := remove(dbPath); err != nil {
		return errors.Annotatef(err, "cannot recursively remove %q", dbPath)
	}
	if err := mkdir(dbPath, fi.Mode()); err != nil {
		return errors.Annotatef(err, "cannot re-create %q", dbPath)
	}
	return nil
}

func (u *UpgradeMongoCommand) removeOldDb(dataDir string) error {
	return removeOldDbCall(dataDir, u.stat, u.remove, u.mkdir)
}

func satisfyPrerequisites(operatingsystem string) error {
	// CentOS is not currently supported by our mongo package.
	if operatingsystem == "centos7" {
		return errors.New("centos7 is still not suported by this upgrade")
	}

	pacman, err := manager.NewPackageManager(operatingsystem)
	if err != nil {
		return errors.Annotatef(err, "cannot obtain package manager for %q", operatingsystem)
	}

	if err := pacman.InstallPrerequisite(); err != nil {
		return err
	}

	if err := pacman.Install("juju-mongodb2.6"); err != nil {
		return errors.Annotate(err, "cannot install juju-mongodb2.6")
	}
	if err := pacman.Install("juju-mongodb3"); err != nil {
		return errors.Annotate(err, "cannot install juju-mongodb3")
	}
	return nil
}

func mongoDumpCall(runCommand utilsRun, tmpDir, mongoPath, adminPassword, migrationName string, statePort int) (string, error) {
	mongodump := path.Join(mongoPath, "mongodump")
	dumpParams := []string{
		"--ssl",
		"-u", "admin",
		"-p", adminPassword,
		"--port", strconv.Itoa(statePort),
		"--host", "localhost",
		"--out", path.Join(tmpDir, fmt.Sprintf("migrateTo%sdump", migrationName)),
	}
	var out string
	var err error
	for attempt := connectAfterRestartRetryAttempts.Start(); attempt.Next(); {
		out, err = runCommand(mongodump, dumpParams...)
		if err == nil {
			break
		}
		logger.Errorf(out)
	}
	if err != nil {
		logger.Errorf(out)
		return out, errors.Annotate(err, "cannot dump mongo db")
	}
	return out, nil
}

func (u *UpgradeMongoCommand) mongoDump(mongoPath, adminPassword, migrationName string, statePort int) (string, error) {
	return mongoDumpCall(u.runCommand, u.tmpDir, mongoPath, adminPassword, migrationName, statePort)
}

func mongoRestoreCall(runCommand utilsRun, tmpDir, mongoPath, adminPassword, migrationName string,
	dbs []string, statePort int, invalidSSL bool, batchSize int) error {
	mongorestore := path.Join(mongoPath, "mongorestore")
	restoreParams := []string{
		"--ssl",
		"--port", strconv.Itoa(statePort),
		"--host", "localhost",
	}

	if invalidSSL {
		restoreParams = append(restoreParams, "--sslAllowInvalidCertificates")
	}
	if batchSize > 0 {
		restoreParams = append(restoreParams, "--batchSize", strconv.Itoa(batchSize))
	}
	if adminPassword != "" {
		restoreParams = append(restoreParams, "-u", "admin", "-p", adminPassword)
	}
	var out string
	var err error
	if len(dbs) == 0 || dbs == nil {
		restoreParams = append(restoreParams, path.Join(tmpDir, fmt.Sprintf("migrateTo%sdump", migrationName)))
		for attempt := connectAfterRestartRetryAttempts.Start(); attempt.Next(); {
			out, err = runCommand(mongorestore, restoreParams...)
			if err == nil {
				break
			}
			logger.Errorf("cannot restore %v: %s", err, out)
		}
		// if err is nil, Annotate returns nil
		return errors.Annotatef(err, "cannot restore dbs got: %s", out)
	}
	for i := range dbs {
		restoreDbParams := append(restoreParams,
			fmt.Sprintf("--db=%s", dbs[i]),
			path.Join(tmpDir, fmt.Sprintf("migrateTo%sdump/%s", migrationName, dbs[i])))
		for attempt := connectAfterRestartRetryAttempts.Start(); attempt.Next(); {
			out, err = runCommand(mongorestore, restoreDbParams...)
			if err == nil {
				break
			}
			logger.Errorf("cannot restore db %q: %v: got %s", dbs[i], err, out)
		}
		if err != nil {
			return errors.Annotatef(err, "cannot restore db %q got: %s", dbs[i], out)
		}
		logger.Infof("Succesfully restored db %q", dbs[i])
	}
	return nil
}

func (u *UpgradeMongoCommand) mongoRestore(mongoPath, adminPassword, migrationName string, dbs []string, statePort int, invalidSSL bool, batchSize int) error {
	return mongoRestoreCall(u.runCommand, u.tmpDir, mongoPath, adminPassword, migrationName, dbs, statePort, invalidSSL, batchSize)
}

// copyBackupMongo will make a copy of mongo db by copying the db
// directory, this is safer than a dump.
func (u *UpgradeMongoCommand) copyBackupMongo(targetVersion, dataDir string) (string, error) {
	tmpDir, err := u.createTempDir()
	if err != nil {
		return "", errors.Annotate(err, "cannot create a working directory for backing up mongo")
	}
	if err := u.mongoStop(); err != nil {
		return "", errors.Annotate(err, "cannot stop mongo to backup")
	}
	defer u.mongoStart()

	dbPath := path.Join(dataDir, "db")
	fi, err := u.stat(dbPath)

	target := path.Join(tmpDir, targetVersion)
	if err := u.mkdir(target, fi.Mode()); err != nil {
		return "", errors.Annotate(err, "cannot create target folder for backup")
	}
	targetDb := path.Join(target, "db")

	u.agentConfig.SetValue(KeyUpgradeBackup, targetDb)
	if err := u.agentConfig.Write(); err != nil {
		return "", errors.Annotate(err, "cannot write agent config backup information")
	}

	if err := u.fsCopy(dbPath, targetDb); err != nil {
		// TODO, delete what was copied
		return "", errors.Annotate(err, "cannot backup mongo database")
	}
	return targetDb, nil
}

func (u *UpgradeMongoCommand) rollbackCopyBackup(dataDir, origin string) error {
	if err := u.mongoStop(); err != nil {
		return errors.Annotate(err, "cannot stop mongo to rollback")
	}
	defer u.mongoStart()

	dbDir := path.Join(dataDir, "db")
	if err := u.remove(dbDir); err != nil {
		return errors.Annotate(err, "could not remove the existing folder to rollback")
	}

	if err := fs.Copy(origin, dbDir); err != nil {
		return errors.Annotate(err, "cannot rollback mongo database")
	}
	if err := u.rollbackAgentConfig(); err != nil {
		return errors.Annotate(err, "cannot roo back agent configuration")
	}
	return errors.Annotate(u.UpdateService(true), "cannot rollback service script")
}

// rollbackAgentconfig rolls back the config value for mongo version
// to its original one and corrects the entry in stop mongo until.
func (u *UpgradeMongoCommand) rollbackAgentConfig() error {
	u.agentConfig.SetMongoVersion(mongo.Mongo24)
	return errors.Annotate(u.agentConfig.Write(), "could not rollback mongo version in agent.config")
}

func enoughFreeSpace() bool {
	return true
}

func (u *UpgradeMongoCommand) upgradeSlave(dataDir string) error {
	if err := u.satisfyPrerequisites(u.series); err != nil {
		return errors.Annotate(err, "cannot satisfy pre-requisites for the migration")
	}
	if err := u.mongoStop(); err != nil {
		return errors.Annotate(err, "cannot stop mongo to upgrade mongo slave")
	}
	defer u.mongoStart()
	if err := u.removeOldDb(dataDir); err != nil {
		return errors.Annotate(err, "cannot remove existing slave db")
	}
	// Mongo 3, with wired tiger
	u.agentConfig.SetMongoVersion(mongo.Mongo30wt)
	if err := u.agentConfig.Write(); err != nil {
		return errors.Annotate(err, "could not update mongo version in agent.config")
	}
	logger.Infof("wired tiger set in agent.config")

	if err := u.UpdateService(false); err != nil {
		return errors.Annotate(err, "cannot update service script to use wired tiger")
	}
	logger.Infof("service startup script up to date")
	return nil
}
