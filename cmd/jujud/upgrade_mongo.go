// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"net"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/packaging/manager"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gnuflag"
)

// NewUpgradeMongoCommand returns a new UpogradeMongo command initialized
func NewUpgradeMongoCommand() *UpgradeMongoCommand {
	return &UpgradeMongoCommand{
		stat:                 os.Stat,
		rename:               os.Rename,
		mkdir:                os.Mkdir,
		runCommand:           utils.RunCommand,
		dialAndLogin:         dialAndLogin,
		satisfyPrerequisites: satisfyPrerequisites,

		mongoStart:                  mongo.StartService,
		mongoStop:                   mongo.StopService,
		mongoRestart:                mongo.ReStartService,
		mongoEnsureServiceInstalled: mongo.EnsureServiceInstalled,
		mongoDialInfo:               mongo.DialInfo,
		initiateMongoServer:         peergrouper.InitiateMongoServer,
	}
}

type statFunc func(string) (os.FileInfo, error)
type renameFunc func(string, string) error
type mkdirFunc func(string, os.FileMode) error

type utilsRun func(command string, args ...string) (output string, err error)

type mgoSession interface {
	Close()
}

type mgoDb interface {
	Run(interface{}, interface{}) error
}

type dialAndLogger func(*mongo.MongoInfo) (mgoSession, mgoDb, error)

type requisitesSatisfier func(string) error

type mongoService func(string) error
type mongoEnsureService func(string, string, int, int, bool, mongo.Version, bool) error
type mongoDialInfo func(mongo.Info, mongo.DialOpts) (*mgo.DialInfo, error)

type initiateMongoServerFunc func(peergrouper.InitiateMongoParams, bool) error

// UpgradeMongoCommand represents a jujud upgrade-mongo command.
type UpgradeMongoCommand struct {
	cmd.CommandBase
	machineTag     string
	series         string
	namespace      string
	configFilePath string
	agentConfig    agent.ConfigSetterWriter

	// utils used by this struct.
	stat                 statFunc
	rename               renameFunc
	mkdir                mkdirFunc
	runCommand           utilsRun
	dialAndLogin         dialAndLogger
	satisfyPrerequisites requisitesSatisfier

	// mongo related utils.
	mongoStart                  mongoService
	mongoStop                   mongoService
	mongoRestart                mongoService
	mongoEnsureServiceInstalled mongoEnsureService
	mongoDialInfo               mongoDialInfo
	initiateMongoServer         initiateMongoServerFunc
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
	f.StringVar(&u.machineTag, "machinetag", "0", "unique tag identifier for machine to be upgraded")
	f.StringVar(&u.series, "series", "", "series for the machine")
	f.StringVar(&u.configFilePath, "configfile", "", "path to the config file")
	f.StringVar(&u.namespace, "namespace", "", "namespace, this should be blank unless the provider is local")

}

// Init initializes the command for running.
func (u *UpgradeMongoCommand) Init(args []string) error {
	return nil
}

// Run migrates an environment to mongo 3.
func (u *UpgradeMongoCommand) Run(ctx *cmd.Context) error {
	return u.run()
}

func (u *UpgradeMongoCommand) run() error {
	logger.Infof("begin migration to mongo 3")
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

	if err := u.satisfyPrerequisites(u.series); err != nil {
		return errors.Annotate(err, "cannot satisfy pre-requisites for the migration")
	}
	if err := u.maybeUpgrade24to26(dataDir); err != nil {
		//TODO (perrito666) add rollback here
		return errors.Annotate(err, "cannot upgrade from mongo 2.4 to 2.6")
	}
	//TODO AGENT CONFIG IS NOT WRITEN
	if err := u.maybeUpgrade26to31(dataDir); err != nil {
		//TODO (perrito666) add rollback here
		return errors.Annotate(err, "cannot upgrade from mongo 2.6 to 3")
	}
	return nil
}

// UpdateService will re-write the service scripts for mongo and restart it.
func (u *UpgradeMongoCommand) UpdateService(namespace string, auth bool) error {
	// TODO(perrito666) This ignores NumaControlPolicy
	// and also oplog size
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
		namespace,
		ssi.StatePort,
		oplogSize,
		numaCtlPolicy,
		u.agentConfig.MongoVersion(),
		auth)
	return errors.Annotate(err, "cannot ensure mongodb service script is properly installed")
}

func (u *UpgradeMongoCommand) maybeUpgrade24to26(dataDir string) error {
	namespace := u.agentConfig.Value(agent.Namespace)
	jujuMongoPath := path.Dir(mongo.JujuMongodPath)
	password := u.agentConfig.OldPassword()
	ssi, _ := u.agentConfig.StateServingInfo()
	port := ssi.StatePort

	logger.Infof("backing up 2.4 MongoDB")
	// TODO(perrito666) dont ignore out if debug-log was used.
	_, err := u.mongoDump(jujuMongoPath, password, "26", port)
	if err != nil {
		return errors.Annotate(err, "could not do pre migration backup")
	}

	//TODO(perrito666) figure namespace.
	//By this point we assume that juju is no longer writing
	// nor reading from the state server.
	logger.Infof("stopping 2.4 MongoDB")
	if err := u.mongoStop(namespace); err != nil {
		return errors.Annotate(err, "cannot stop mongo to perform 2.6 upgrade step")
	}

	// Run the optional --upgrade step on mongodb 2.6.
	if err := u.mongo26UpgradeStep(dataDir); err != nil {
		return errors.Annotate(err, "cannot run mongo 2.6 with --upgrade")
	}

	u.agentConfig.SetMongoVersion(mongo.Mongo26)
	if err := u.agentConfig.Write(); err != nil {
		return errors.Annotate(err, "could not update mongo version in agent.config")
	}

	if err := u.UpdateService(namespace, true); err != nil {
		return errors.Annotate(err, "cannot update mongo service to use mongo 2.6")
	}

	logger.Infof("starting 2.6 MongoDB")
	if err := u.mongoStart(namespace); err != nil {
		return errors.Annotate(err, "cannot start mongo 2.6 to upgrade auth schema")
	}

	info, ok := u.agentConfig.MongoInfo()
	if !ok {
		return errors.New("cannot get mongo info from agent config")
	}

	session, db, err := u.dialAndLogin(info)
	defer session.Close()
	if err != nil {
		return errors.Annotate(err, "error dialing mongo to uprade auth schema")
	}

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
	if err := u.mongoRestart(namespace); err != nil {
		return errors.Annotate(err, "cannot restart mongodb 2.6 service")
	}
	// TODO(perrito666), if anything failed, mongo version should be rolled back
	// and dump re-loaded into the old db path.
	return nil
}

func (u *UpgradeMongoCommand) maybeUpgrade26to31(dataDir string) error {
	namespace := u.agentConfig.Value(agent.Namespace)
	jujuMongoPath := path.Dir(mongo.JujuMongod26Path)
	password := u.agentConfig.OldPassword()
	ssi, _ := u.agentConfig.StateServingInfo()
	port := ssi.StatePort

	logger.Infof("backing up 2.6 MongoDB")
	// TODO(perrito666) dont ignore out if debug-log was used.
	_, err := u.mongoDump(jujuMongoPath, password, "30", port)
	if err != nil {
		return errors.Annotate(err, "could not do pre migration backup")
	}
	if err := u.mongoStop(namespace); err != nil {
		return errors.Annotate(err, "cannot stop mongo to update to mongo 3")
	}

	// REWRITE SERVICE

	// Mongo 3, no wired tiger
	u.agentConfig.SetMongoVersion(mongo.Mongo30)
	if err := u.agentConfig.Write(); err != nil {
		return errors.Annotate(err, "could not update mongo version in agent.config")
	}

	if err := u.UpdateService(namespace, true); err != nil {
		return errors.Annotate(err, "cannot update service script")
	}

	// TODO: this one is not working.
	if err := u.mongoStart(namespace); err != nil {
		return errors.Annotate(err, "cannot start mongo 3 to do a pre-tiger migration dump")
	}

	jujuMongoPath = path.Dir(mongo.JujuMongod30Path)
	_, err = u.mongoDump(jujuMongoPath, password, "Tiger", port)
	if err != nil {
		return errors.Annotate(err, "could not do the tiger migration export")
	}

	if err := u.mongoStop(namespace); err != nil {
		return errors.Annotate(err, "cannot stop mongo to update to wired tiger")
	}
	if err := u.renameOldDb(u.agentConfig.DataDir()); err != nil {
		return errors.Annotate(err, "cannot prepare the new db location for wired tiger")
	}

	// Mongo 3, with wired tiger
	u.agentConfig.SetMongoVersion(mongo.Mongo30wt)
	if err := u.agentConfig.Write(); err != nil {
		return errors.Annotate(err, "could not update mongo version in agent.config")
	}

	if err := u.UpdateService(namespace, false); err != nil {
		return errors.Annotate(err, "cannot update service script to use wired tiger")
	}

	info, ok := u.agentConfig.MongoInfo()
	if !ok {
		return errors.New("cannot get mongo info from agent config")
	}

	//TODO(perrito666) make this into its own function
	dialOpts := mongo.DialOpts{}
	dialInfo, err := u.mongoDialInfo(info.Info, dialOpts)
	if err != nil {
		return errors.Annotate(err, "cannot obtain dial info")
	}

	if err := u.mongoStart(namespace); err != nil {
		return errors.Annotate(err, "cannot start mongo 3 to restart replicaset")
	}

	// perhaps statehost port?
	// we need localhost, since there is no admin user
	peerHostPort := net.JoinHostPort("localhost", fmt.Sprint(ssi.StatePort))
	// TODO(perrito666) this should be using noauth so we can so stuff
	// or not, perhaps the no admin localhost only thing works
	err = u.initiateMongoServer(peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: peerHostPort,
	}, true)
	if err != nil {
		return errors.Annotate(err, "cannot initiate replicaset")
	}

	// TODO (perrito666) need to figure out why blobstore is not restoreable.
	err = u.mongoRestore(jujuMongoPath, "", "Tiger", []string{"juju", "admin", "logs", "presence"}, port)
	if err != nil {
		return errors.Annotate(err, "cannot restore the db.")
	}

	if err := u.UpdateService(namespace, true); err != nil {
		return errors.Annotate(err, "cannot update service script post wired tiger migration")
	}

	if err := u.mongoRestart(namespace); err != nil {
		return errors.Annotate(err, "cannot restart mongo service after upgrade")
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

func renameOldDbCall(dataDir string, stat statFunc, rename renameFunc, mkdir mkdirFunc) error {
	dbPath := path.Join(dataDir, "db")
	bkpDbPath := path.Join(dataDir, "db.old")
	fi, err := stat(dbPath)
	if err != nil {
		return errors.Annotatef(err, "cannot stat %q", dbPath)
	}
	if err := rename(dbPath, bkpDbPath); err != nil {
		return errors.Annotatef(err, "cannot mv %q to %q", dbPath, bkpDbPath)
	}
	if err := mkdir(dbPath, fi.Mode()); err != nil {
		return errors.Annotatef(err, "cannot re-create %q", dbPath)
	}
	return nil
}

func (u *UpgradeMongoCommand) renameOldDb(dataDir string) error {
	return renameOldDbCall(dataDir, u.stat, u.rename, u.mkdir)
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
	if err := pacman.AddRepository("ppa:hduran-8/juju-mongodb2.6"); err != nil {
		return errors.Annotate(err, "cannot add ppa for mongo 2.6")
	}
	if err := pacman.AddRepository("ppa:hduran-8/juju-mongodb3"); err != nil {
		return errors.Annotate(err, "cannot add ppa for mongo 3")
	}
	if err := pacman.Update(); err != nil {
		return errors.Annotate(err, "cannot update package package db")
	}

	if err := pacman.Install("juju-mongodb2.6"); err != nil {
		return errors.Annotate(err, "cannot install juju-mongodb2.6")
	}
	if err := pacman.Install("juju-mongodb3"); err != nil {
		return errors.Annotate(err, "cannot install juju-mongodb3")
	}
	return nil
}

func mongoDumpCall(runCommand utilsRun, mongoPath, adminPassword, migrationName string, statePort int) (string, error) {
	mongodump := path.Join(mongoPath, "mongodump")
	dumpParams := []string{
		"--ssl",
		"-u", "admin",
		"-p", adminPassword,
		"--port", strconv.Itoa(statePort),
		"--host", "localhost",
		"--out", fmt.Sprintf("/home/ubuntu/migrateTo%sdump", migrationName),
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
	return mongoDumpCall(u.runCommand, mongoPath, adminPassword, migrationName, statePort)
}

func mongoRestoreCall(runCommand utilsRun, mongoPath, adminPassword, migrationName string, dbs []string, statePort int) error {
	mongorestore := path.Join(mongoPath, "mongorestore")
	restoreParams := []string{
		"--ssl",
		"--sslAllowInvalidCertificates",
		"--port", strconv.Itoa(statePort),
		"--host", "localhost",
	}
	if adminPassword != "" {
		restoreParams = append(restoreParams, "-u", "admin", "-p", adminPassword)
	}
	var out string
	var err error
	if len(dbs) == 0 {
		restoreParams = append(restoreParams, fmt.Sprintf("/home/ubuntu/migrateTo%sdump", migrationName))
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
			fmt.Sprintf("/home/ubuntu/migrateTo%sdump/%s", migrationName, dbs[i]))
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

func (u *UpgradeMongoCommand) mongoRestore(mongoPath, adminPassword, migrationName string, dbs []string, statePort int) error {
	return mongoRestoreCall(u.runCommand, mongoPath, adminPassword, migrationName, dbs, statePort)
}
