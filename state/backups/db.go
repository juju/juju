// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/imagestorage"
)

// db is a surrogate for the proverbial DB layer abstraction that we
// wish we had for juju state.  To that end, the package holds the DB
// implementation-specific details and functionality needed for backups.
// Currently that means mongo-specific details.  However, as a stand-in
// for a future DB layer abstraction, the db package does not expose any
// low-level details publicly.  Thus the backups implementation remains
// oblivious to the underlying DB implementation.

var runCommandFn = runCommand

// DBInfo wraps all the DB-specific information backups needs to dump
// the database. This includes a simplification of the information in
// authentication.MongoInfo.
type DBInfo struct {
	// Address is the DB system's host address.
	Address string
	// Username is used when connecting to the DB system.
	Username string
	// Password is used when connecting to the DB system.
	Password string
	// Targets is a list of databases to dump.
	Targets set.Strings
	// MongoVersion the version of the running mongo db.
	MongoVersion mongo.Version
}

// ignoredDatabases is the list of databases that should not be
// backed up, admin might be removed later, after determining
// mongo version.
var ignoredDatabases = set.NewStrings(
	"admin",
	storageDBName,
	"presence",            // note: this is still backed up anyway
	imagestorage.ImagesDB, // note: this is still backed up anyway
)

type DBSession interface {
	DatabaseNames() ([]string, error)
}

// NewDBInfo returns the information needed by backups to dump
// the database.
func NewDBInfo(mgoInfo *mongo.MongoInfo, session DBSession, version mongo.Version) (*DBInfo, error) {
	targets, err := getBackupTargetDatabases(session)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info := DBInfo{
		Address:      mgoInfo.Addrs[0],
		Password:     mgoInfo.Password,
		Targets:      targets,
		MongoVersion: version,
	}

	// TODO(dfc) Backup should take a Tag.
	if mgoInfo.Tag != nil {
		info.Username = mgoInfo.Tag.String()
	}

	return &info, nil
}

func getBackupTargetDatabases(session DBSession) (set.Strings, error) {
	dbNames, err := session.DatabaseNames()
	if err != nil {
		return nil, errors.Annotate(err, "unable to get DB names")
	}

	targets := set.NewStrings(dbNames...).Difference(ignoredDatabases)
	return targets, nil
}

const (
	dumpName    = "mongodump"
	restoreName = "mongorestore"
)

// DBDumper is any type that dumps something to a dump dir.
type DBDumper interface {
	// Dump something to dumpDir.
	Dump(dumpDir string) error
}

var getMongodumpPath = func() (string, error) {
	return getMongoToolPath(dumpName, os.Stat, exec.LookPath)
}

var getMongodPath = func() (string, error) {
	return mongo.Path(mongo.InstalledVersion())
}

func getMongoToolPath(toolName string, stat func(name string) (os.FileInfo, error), lookPath func(file string) (string, error)) (string, error) {
	mongod, err := getMongodPath()
	if err != nil {
		return "", errors.Annotate(err, "failed to get mongod path")
	}
	mongoTool := filepath.Join(filepath.Dir(mongod), toolName)

	if _, err := stat(mongoTool); err == nil {
		// It already exists so no need to continue.
		return mongoTool, nil
	}

	path, err := lookPath(toolName)
	if err != nil {
		return "", errors.Trace(err)
	}
	return path, nil
}

type mongoDumper struct {
	*DBInfo
	// binPath is the path to the dump executable.
	binPath string
}

// NewDBDumper returns a new value with a Dump method for dumping the
// juju state database.
func NewDBDumper(info *DBInfo) (DBDumper, error) {
	mongodumpPath, err := getMongodumpPath()
	if err != nil {
		return nil, errors.Annotate(err, "mongodump not available")
	}

	dumper := mongoDumper{
		DBInfo:  info,
		binPath: mongodumpPath,
	}
	return &dumper, nil
}

func (md *mongoDumper) options(dumpDir string) []string {
	options := []string{
		"--ssl",
		"--authenticationDatabase", "admin",
		"--host", md.Address,
		"--username", md.Username,
		"--password", md.Password,
		"--out", dumpDir,
		"--oplog",
	}
	return options
}

func (md *mongoDumper) dump(dumpDir string) error {
	options := md.options(dumpDir)
	if err := runCommandFn(md.binPath, options...); err != nil {
		return errors.Annotate(err, "error dumping databases")
	}
	return nil
}

// Dump dumps the juju state-related databases.  To do this we dump all
// databases and then remove any ignored databases from the dump results.
func (md *mongoDumper) Dump(baseDumpDir string) error {
	if err := md.dump(baseDumpDir); err != nil {
		return errors.Trace(err)
	}

	found, err := listDatabases(baseDumpDir)
	if err != nil {
		return errors.Trace(err)
	}

	// Strip the ignored database from the dump dir.
	ignored := found.Difference(md.Targets)
	// Admin must be removed only if the mongo version is 3.x or
	// above, since 2.x will not restore properly without admin.
	if md.DBInfo.MongoVersion.NewerThan(mongo.Mongo26) == -1 {
		ignored.Remove("admin")
	}
	err = stripIgnored(ignored, baseDumpDir)
	return errors.Trace(err)
}

// stripIgnored removes the ignored DBs from the mongo dump files.
// This involves deleting DB-specific directories.
//
// NOTE(fwereade): the only directories we actually delete are "admin"
// and "backups"; and those only if they're in the `ignored` set. I have
// no idea why the code was structured this way; but I am, as requested
// as usual by management, *not* fixing anything about backup beyond the
// bug du jour.
//
// Basically, the ignored set is a filthy lie, and all the work we do to
// generate it is pure obfuscation.
func stripIgnored(ignored set.Strings, dumpDir string) error {
	for _, dbName := range ignored.Values() {
		switch dbName {
		case storageDBName, "admin":
			dirname := filepath.Join(dumpDir, dbName)
			if err := os.RemoveAll(dirname); err != nil {
				return errors.Trace(err)
			}
		}
	}

	return nil
}

// listDatabases returns the name of each sub-directory of the dump
// directory.  Each corresponds to a database dump generated by
// mongodump.  Note that, while mongodump is unlikely to change behavior
// in this regard, this is not a documented guaranteed behavior.
func listDatabases(dumpDir string) (set.Strings, error) {
	list, err := ioutil.ReadDir(dumpDir)
	if err != nil {
		return set.Strings{}, errors.Trace(err)
	}

	databases := make(set.Strings)
	for _, info := range list {
		if !info.IsDir() {
			// Notably, oplog.bson is thus excluded here.
			continue
		}
		databases.Add(info.Name())
	}
	return databases, nil
}

var getMongorestorePath = func() (string, error) {
	return getMongoToolPath(restoreName, os.Stat, exec.LookPath)
}

// DBDumper is any type that dumps something to a dump dir.
type DBRestorer interface {
	// Dump something to dumpDir.
	Restore(dumpDir string, dialInfo *mgo.DialInfo) error
}

type mongoRestorer struct {
	*mgo.DialInfo
	// binPath is the path to the dump executable.
	binPath         string
	tagUser         string
	tagUserPassword string
	runCommandFn    func(string, ...string) error
}
type mongoRestorer32 struct {
	mongoRestorer
	getDB           func(string, MongoSession) MongoDB
	newMongoSession func(*mgo.DialInfo) (MongoSession, error)
}

type mongoRestorer24 struct {
	mongoRestorer
	stopMongo  func() error
	startMongo func() error
}

func (md *mongoRestorer24) options(dumpDir string) []string {
	dbDir := filepath.Join(agent.DefaultPaths.DataDir, "db")
	options := []string{
		"--drop",
		"--journal",
		"--oplogReplay",
		"--dbpath", dbDir,
		dumpDir,
	}
	return options
}

func (md *mongoRestorer24) Restore(dumpDir string, _ *mgo.DialInfo) error {
	logger.Debugf("stopping mongo service for restore")
	if err := md.stopMongo(); err != nil {
		return errors.Annotate(err, "cannot stop mongo to replace files")
	}
	options := md.options(dumpDir)
	logger.Infof("restoring database with params %v", options)
	if err := md.runCommandFn(md.binPath, options...); err != nil {
		return errors.Annotate(err, "error restoring database")
	}
	if err := md.startMongo(); err != nil {
		return errors.Annotate(err, "cannot start mongo after restore")
	}

	return nil
}

// GetDB wraps mgo.Session.DB to ease testing.
func GetDB(s string, session MongoSession) MongoDB {
	return session.DB(s)
}

// NewMongoSession wraps mgo.DialInfo to ease testing.
func NewMongoSession(dialInfo *mgo.DialInfo) (MongoSession, error) {
	return mgo.DialWithInfo(dialInfo)
}

type RestorerArgs struct {
	DialInfo        *mgo.DialInfo
	NewMongoSession func(*mgo.DialInfo) (MongoSession, error)
	Version         mongo.Version
	TagUser         string
	TagUserPassword string
	GetDB           func(string, MongoSession) MongoDB

	RunCommandFn func(string, ...string) error
	StartMongo   func() error
	StopMongo    func() error
}

var mongoInstalledVersion = mongo.InstalledVersion

// NewDBRestorer returns a new structure that can perform a restore
// on the db pointed in dialInfo.
func NewDBRestorer(args RestorerArgs) (DBRestorer, error) {
	mongorestorePath, err := getMongorestorePath()
	if err != nil {
		return nil, errors.Annotate(err, "mongorestrore not available")
	}

	installedMongo := mongoInstalledVersion()
	// NewerThan will check Major and Minor so migration between micro versions
	// will work, before changing this bewar, Mongo has been known to break
	// compatibility between minors.
	if args.Version.NewerThan(installedMongo) != 0 {
		return nil, errors.NotSupportedf("restore mongo version %s into version %s", args.Version.String(), installedMongo.String())
	}

	var restorer DBRestorer
	mgoRestorer := mongoRestorer{
		DialInfo:        args.DialInfo,
		binPath:         mongorestorePath,
		tagUser:         args.TagUser,
		tagUserPassword: args.TagUserPassword,
		runCommandFn:    args.RunCommandFn,
	}
	switch args.Version.Major {
	case 2:
		restorer = &mongoRestorer24{
			mongoRestorer: mgoRestorer,
			startMongo:    args.StartMongo,
			stopMongo:     args.StopMongo,
		}
	case 3:
		restorer = &mongoRestorer32{
			mongoRestorer:   mgoRestorer,
			getDB:           args.GetDB,
			newMongoSession: args.NewMongoSession,
		}
	default:
		return nil, errors.Errorf("cannot restore from mongo version %q", args.Version.String())
	}
	return restorer, nil
}

func (md *mongoRestorer32) options(dumpDir string) []string {
	// note the batchSize, which is known to mitigate EOF errors
	// seen when using mongorestore; as seen and reported in
	// https://jira.mongodb.org/browse/TOOLS-939 -- not guaranteed
	// to *help* with lp:1605653, but observed not to hurt.
	//
	// The value of 100 was chosen because it's more pessimistic
	// than the "1000" that many report success using in the bug.
	options := []string{
		"--ssl",
		"--authenticationDatabase", "admin",
		"--host", md.Addrs[0],
		"--username", md.Username,
		"--password", md.Password,
		"--drop",
		"--oplogReplay",
		"--batchSize", "100",
		dumpDir,
	}
	return options
}

// MongoDB represents a mgo.DB.
type MongoDB interface {
	UpsertUser(*mgo.User) error
}

// MongoSession represents mgo.Session.
type MongoSession interface {
	Run(cmd interface{}, result interface{}) error
	Close()
	DB(string) *mgo.Database
}

// ensureOplogPermissions adds a special role to the admin user, this role
// is required by mongorestore when doing oplogreplay.
func (md *mongoRestorer32) ensureOplogPermissions(dialInfo *mgo.DialInfo) error {
	s, err := md.newMongoSession(dialInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer s.Close()

	roles := bson.D{
		{"createRole", "oploger"},
		{"privileges", []bson.D{
			bson.D{
				{"resource", bson.M{"anyResource": true}},
				{"actions", []string{"anyAction"}},
			},
		}},
		{"roles", []string{}},
	}
	var mgoErr bson.M
	err = s.Run(roles, &mgoErr)
	if err != nil {
		return errors.Trace(err)
	}
	result, ok := mgoErr["ok"]
	success, isFloat := result.(float64)
	if (!ok || !isFloat || success != 1) && mgoErr != nil {
		return errors.Errorf("could not create special role to replay oplog, result was: %#v", mgoErr)
	}

	// This will replace old user with the new credentials
	admin := md.getDB("admin", s)

	grant := bson.D{
		{"grantRolesToUser", md.DialInfo.Username},
		{"roles", []string{"oploger"}},
	}

	err = s.Run(grant, &mgoErr)
	if err != nil {
		return errors.Trace(err)
	}
	result, ok = mgoErr["ok"]
	success, isFloat = result.(float64)
	if (!ok || !isFloat || success != 1) && mgoErr != nil {
		return errors.Errorf("could not grant special role to %q, result was: %#v", md.DialInfo.Username, mgoErr)
	}

	grant = bson.D{
		{"grantRolesToUser", "admin"},
		{"roles", []string{"oploger"}},
	}

	err = s.Run(grant, &mgoErr)
	if err != nil {
		return errors.Trace(err)
	}
	result, ok = mgoErr["ok"]
	success, isFloat = result.(float64)
	if (!ok || !isFloat || success != 1) && mgoErr != nil {
		return errors.Errorf("could not grant special role to \"admin\", result was: %#v", mgoErr)
	}

	if err := admin.UpsertUser(&mgo.User{
		Username: md.DialInfo.Username,
		Password: md.DialInfo.Password,
	}); err != nil {
		return errors.Errorf("cannot set new admin credentials: %v", err)
	}

	return nil
}

func (md *mongoRestorer32) ensureTagUser() error {
	s, err := md.newMongoSession(md.DialInfo)
	if err != nil {
		return errors.Trace(err)
	}
	defer s.Close()

	admin := md.getDB("admin", s)

	if err := admin.UpsertUser(&mgo.User{
		Username: md.tagUser,
		Password: md.tagUserPassword,
	}); err != nil {
		return fmt.Errorf("cannot set tag user credentials: %v", err)
	}
	return nil
}

func (md *mongoRestorer32) Restore(dumpDir string, dialInfo *mgo.DialInfo) error {
	if err := md.ensureOplogPermissions(dialInfo); err != nil {
		return errors.Annotate(err, "setting special user permission in db")
	}

	options := md.options(dumpDir)
	logger.Infof("restoring database with params %v", options)
	if err := md.runCommandFn(md.binPath, options...); err != nil {
		return errors.Annotate(err, "error restoring database")
	}
	logger.Infof("updating user credentials")
	if err := md.ensureTagUser(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
