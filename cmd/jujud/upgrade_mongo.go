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
	"launchpad.net/gnuflag"
)

// NewUpgradeMongoCommand returns a new UpogradeMongo command initialized
func NewUpgradeMongoCommand() *UpgradeMongoCommand {
	return &UpgradeMongoCommand{}
}

// UpgradeMongoCommand represents a jujud upgrade-mongo command.
type UpgradeMongoCommand struct {
	cmd.CommandBase
	machineTag     string
	series         string
	namespace      string
	configFilePath string
	agentConfig    agent.ConfigSetterWriter
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

	if err := satisfyPrerequisites(u.series); err != nil {
		return errors.Annotate(err, "cannot satisfy pre-requisites for the migration")
	}
	if err := u.maybeUpgrade24to26(dataDir); err != nil {
		//TODO (perrito666) add rollback here
		return errors.Annotate(err, "cannot upgrade from mongo 2.4 to 2.6")
	}
	return nil
}

func satisfyPrerequisites(operatingsystem string) error {
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

	// CentOS is not currently supported by our mongo package.
	if operatingsystem == "centos7" {
		return errors.New("centos7 is still not suported by this upgrade")
	}
	if err := pacman.Install("juju-mongodb2.6"); err != nil {
		return errors.Annotate(err, "cannot install juju-mongodb2.6")
	}
	if err := pacman.Install("juju-mongodb3"); err != nil {
		return errors.Annotate(err, "cannot install juju-mongodb3")
	}
	return nil
}

func mongoDump(mongoPath, adminPassword, migrationName string, statePort int) (string, error) {
	mongodump := path.Join(mongoPath, "mongodump")
	dumpParams := []string{
		"--ssl",
		"-u", "admin",
		"-p", utils.ShQuote(adminPassword),
		"--port", strconv.Itoa(statePort),
		"--host", "localhost",
		"--out", fmt.Sprintf("~/migrateTo%sdump", migrationName),
	}
	fmt.Printf("will run mongodump: %q with args %v", mongodump, dumpParams)
	out, err := utils.RunCommand(mongodump, dumpParams...)
	if err != nil {
		logger.Errorf(out)
		return out, errors.Annotate(err, "cannot dump mongo db")
	}
	return out, nil
}

func mongoRestore(mongoPath, adminPassword, migrationName string, dbs []string, statePort int) error {
	mongorestore := path.Join(mongoPath, "mongorestore")
	restoreParams := []string{
		"--ssl",
		"--sslAllowInvalidCertificates",
		"--port", strconv.Itoa(statePort),
		"--host", "localhost",
	}
	if adminPassword != "" {
		restoreParams = append(restoreParams, "-u", "admin", "-p", utils.ShQuote(adminPassword))
	}
	if dbs == nil {
		restoreParams = append(restoreParams, fmt.Sprintf("~/migrateTo%sdump", migrationName))
		_, err := utils.RunCommand(mongorestore, restoreParams...)
		// if err is nil, Annotate returns nil
		return errors.Annotate(err, "cannot restore dbs")
	}
	for i := range dbs {
		restoreDbParams := append(restoreParams,
			fmt.Sprintf("--db=%s", dbs[i]),
			fmt.Sprintf("~/migrateTo%sdump/%s", migrationName, dbs[i]))
		if _, err := utils.RunCommand(mongorestore, restoreDbParams...); err != nil {
			return errors.Annotatef(err, "cannot restore db %q", dbs[i])
		}
		logger.Infof("Succesfully restored db %q", dbs[i])
	}
	return nil

}

func mongo26UpgradeStep(dataDir string) error {
	// /usr/lib/juju/mongo2.6/bin/mongod --dbpath {{.JujuDbPath}} --replSet juju --upgrade
	updateArgs := []string{"--dbpath", mongo.DbDir(dataDir), "--replSet", "juju", "--upgrade"}
	out, err := utils.RunCommand(mongo.JujuMongodPath, updateArgs...)
	logger.Infof(out)
	if err != nil {
		return errors.Annotate(err, "cannot upgrade mongo 2.4 data")
	}
	return nil
}

// UpdateService will re-write the service scripts for mongo and restart it.
func (u *UpgradeMongoCommand) UpdateService(namespace string) error {
	// TODO(perrito666) This ignores NumaControlPolicy
	// and also oplog size
	ssi, _ := u.agentConfig.StateServingInfo()
	params := mongo.EnsureServerParams{
		APIPort:        ssi.APIPort,
		StatePort:      ssi.StatePort,
		Cert:           ssi.Cert,
		PrivateKey:     ssi.PrivateKey,
		CAPrivateKey:   ssi.CAPrivateKey,
		SharedSecret:   ssi.SharedSecret,
		SystemIdentity: ssi.SystemIdentity,
		DataDir:        u.agentConfig.DataDir(),
		Namespace:      namespace,
		Version:        u.agentConfig.MongoVersion(),
	}
	return mongo.EnsureServer(params)
}

// connectAfterRestartRetryAttempts defines how many retries
// will there be when trying to connect to mongo after starting it.
var connectAfterRestartRetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Minute,
	Delay: 1 * time.Second,
}

func (u *UpgradeMongoCommand) maybeUpgrade24to26(dataDir string) error {
	namespace := ""
	jujuMongoPath := path.Dir(mongo.JujuMongodPath)
	password := u.agentConfig.OldPassword()
	ssi, _ := u.agentConfig.StateServingInfo()
	port := ssi.StatePort

	logger.Infof("backing up 2.4 MongoDB")
	// TODO(perrito666) dont ignore out if debug-log was used.
	_, err := mongoDump(jujuMongoPath, password, "26", port)
	if err != nil {
		return errors.Annotate(err, "could not do pre migration backup")
	}

	//TODO(perrito666) figure namespace.
	//By this point we assume that juju is no longer writing
	// nor reading from the state server.
	logger.Infof("stopping 2.4 MongoDB")
	if err := mongo.StopService(namespace); err != nil {
		return errors.Annotate(err, "cannot stop mongo to perform 2.6 upgrade step")
	}

	// Run the optional --upgrade step on mongodb 2.6.
	if err := mongo26UpgradeStep(dataDir); err != nil {
		return errors.Annotate(err, "cannot run mongo 2.6 with --upgrade")
	}

	logger.Infof("stsarting 2.6 MongoDB")
	if err := mongo.StartService(namespace); err != nil {
		return errors.Annotate(err, "cannot start mongo 2.6 to upgrade auth schema")
	}

	info, ok := u.agentConfig.MongoInfo()
	if !ok {
		return errors.New("cannot get mongo info from agent config")
	}
	opts := mongo.DefaultDialOpts()

	var session *mgo.Session
	for attempt := connectAfterRestartRetryAttempts.Start(); attempt.Next(); {
		// Try to connect, retry a few times until the db comes up.
		session, err = mongo.DialWithInfo(info.Info, opts)
		if err == nil {
			break
		}
		logger.Errorf("cannot open mongo connection to upgrade auth schema: %v", err)
	}
	defer session.Close()
	if err != nil {
		return errors.Annotate(err, "timed out trying to upgrade auth schema")
	}
	db := session.DB("admin")
	var res map[string]interface{}
	err = db.Run("authSchemaUpgrade", res)
	if err != nil {
		return errors.Annotate(err, "cannot upgrade auth schema")
	}
	if res["ok"].(int) != 1 {
		return errors.Errorf("cannot upgrade auth schema :%s", res["message"])
	}
	session.Close()
	if err := mongo.ReStartService(namespace); err != nil {
		return errors.Annotate(err, "cannot restart mongodb 2.6 service")
	}
	// finally upgrade the mongo version in agent.conf
	u.agentConfig.SetMongoVersion(mongo.Mongo26)
	if err := u.agentConfig.Write(); err != nil {
		return errors.Annotate(err, "could not update mongo version in agent.config")
	}
	return nil
}

func rollback24to26() error {
	return nil
}

func renameOldDb(dataDir string) error {
	dbPath := path.Join(dataDir, "db")
	bkpDbPath := path.Join(dataDir, "db.old")
	fi, err := os.Stat(dbPath)
	if err != nil {
		return errors.Annotatef(err, "cannot stat %q", dbPath)
	}
	if err := os.Rename(dbPath, bkpDbPath); err != nil {
		return errors.Annotatef(err, "cannot mv %q to %q", dbPath, bkpDbPath)
	}
	if err := os.Mkdir(dbPath, fi.Mode()); err != nil {
		return errors.Annotatef(err, "cannot re-create %q", dbPath)
	}
	return nil
}

func (u *UpgradeMongoCommand) maybeUpgrade26to31(dataDir string) error {
	namespace := ""
	jujuMongoPath := path.Dir(mongo.JujuMongodPath)
	password := u.agentConfig.OldPassword()
	ssi, _ := u.agentConfig.StateServingInfo()
	port := ssi.StatePort

	logger.Infof("backing up 2.6 MongoDB")
	// TODO(perrito666) dont ignore out if debug-log was used.
	_, err := mongoDump(jujuMongoPath, password, "30", port)
	if err != nil {
		return errors.Annotate(err, "could not do pre migration backup")
	}
	if err := mongo.StopService(namespace); err != nil {
		return errors.Annotate(err, "cannot stop mongo to update to mongo 3")
	}

	// REWRITE SERVICE

	// Mongo 3, no wired tiger
	u.agentConfig.SetMongoVersion(mongo.Mongo30)
	if err := u.agentConfig.Write(); err != nil {
		return errors.Annotate(err, "could not update mongo version in agent.config")
	}

	if err := u.UpdateService(namespace); err != nil {
		return errors.Annotate(err, "cannot update service script")
	}

	_, err = mongoDump(jujuMongoPath, password, "Tiger", port)
	if err != nil {
		return errors.Annotate(err, "could not do the tiger migration export")
	}

	if err := mongo.StopService(namespace); err != nil {
		return errors.Annotate(err, "cannot stop mongo to update to wired tiger")
	}
	if err := renameOldDb(u.agentConfig.DataDir()); err != nil {
		return errors.Annotate(err, "cannot prepare the new db location for wired tiger")
	}

	// Mongo 3, with wired tiger
	u.agentConfig.SetMongoVersion(mongo.Mongo30wt)
	if err := u.agentConfig.Write(); err != nil {
		return errors.Annotate(err, "could not update mongo version in agent.config")
	}

	if err := u.UpdateService(namespace); err != nil {
		return errors.Annotate(err, "cannot update service script to use wired tiger")
	}

	info, ok := u.agentConfig.MongoInfo()
	if !ok {
		return errors.New("cannot get mongo info from agent config")
	}

	//TODO(perrito666) make this into its own function
	dialOpts := mongo.DialOpts{}
	dialInfo, err := mongo.DialInfo(info.Info, dialOpts)
	if err != nil {
		return errors.Annotate(err, "cannot obtain dial info")
	}

	// perhaps statehost port?
	// we need localhost, since there is no admin user
	peerHostPort := net.JoinHostPort("localhost", fmt.Sprint(ssi.StatePort))
	// TODO(perrito666) this should be using noauth so we can so stuff
	// or not, perhaps the no admin localhost only thing works
	err = peergrouper.InitiateMongoServer(peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: peerHostPort,
	}, true)
	if err != nil {
		return errors.Annotate(err, "cannot initiate replicaset")
	}

	// TODO (perrito666) need to figure out why blobstore is not restoreable.
	err = mongoRestore(jujuMongoPath, "", "Tiger", []string{"juju", "admin", "logs", "presence"}, port)
	if err != nil {
		return errors.Annotate(err, "cannot restore the db.")
	}

	if err := mongo.ReStartService(namespace); err != nil {
		return errors.Annotate(err, "cannot stop restart mongo service after upgrade")
	}

	return nil
}
