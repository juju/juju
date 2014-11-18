// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/utils"
)

// db is a surrogate for the proverbial DB layer abstraction that we
// wish we had for juju state.  To that end, the package holds the DB
// implementation-specific details and functionality needed for backups.
// Currently that means mongo-specific details.  However, as a stand-in
// for a future DB layer abstraction, the db package does not expose any
// low-level details publicly.  Thus the backups implementation remains
// oblivious to the underlying DB implementation.

var runCommand = utils.RunCommand

// DBConnInfo is a simplification of authentication.MongoInfo, focused
// on the needs of juju state backups.  To ensure that the info is valid
// for use in backups, use the Check() method to get the contained
// values.
type DBConnInfo struct {
	// Address is the DB system's host address.
	Address string
	// Username is used when connecting to the DB system.
	Username string
	// Password is used when connecting to the DB system.
	Password string
}

// Validate checks the DB connection info.  If it isn't valid for use in
// juju state backups, it returns an error.  Make sure that the ConnInfo
// values do not change between the time you call this method and when
// you actually need the values.
func (ci *DBConnInfo) Validate() error {
	if ci.Address == "" {
		return errors.New("missing address")
	}
	if ci.Username == "" {
		return errors.New("missing username")
	}
	if ci.Password == "" {
		return errors.New("missing password")
	}
	return nil
}

// DBInfo wraps all the DB-specific information backups needs to dump
// and restore the database.
type DBInfo struct {
	DBConnInfo
	// Targets is a list of databases to dump.
	Targets set.Strings
}

// ignoredDatabases is the list of databases that should not be
// backed up.
var ignoredDatabases = set.NewStrings(
	"backups",
	"presence",
)

type dbProvider interface {
	// MongoConnectionInfo returns the mongo conn info to use for backups.
	MongoConnectionInfo() *mongo.MongoInfo
	// MongoSession return the session to use for backups.
	MongoSession() *mgo.Session
}

// NewDBBackupInfo returns the information needed by backups to dump
// the database.
func NewDBBackupInfo(st dbProvider) (*DBInfo, error) {
	connInfo := newMongoConnInfo(st.MongoConnectionInfo())
	targets, err := getBackupTargetDatabases(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info := DBInfo{
		DBConnInfo: *connInfo,
		Targets:    targets,
	}
	return &info, nil
}

func newMongoConnInfo(mgoInfo *mongo.MongoInfo) *DBConnInfo {
	info := DBConnInfo{
		Address:  mgoInfo.Addrs[0],
		Password: mgoInfo.Password,
	}

	// TODO(dfc) Backup should take a Tag.
	if mgoInfo.Tag != nil {
		info.Username = mgoInfo.Tag.String()
	}

	return &info
}

func getBackupTargetDatabases(st dbProvider) (set.Strings, error) {
	dbNames, err := st.MongoSession().DatabaseNames()
	if err != nil {
		return nil, errors.Annotate(err, "unable to get DB names")
	}

	targets := set.NewStrings(dbNames...).Difference(ignoredDatabases)
	return targets, nil
}

const dumpName = "mongodump"

// DBDumper is any type that dumps something to a dump dir.
type DBDumper interface {
	// Dump something to dumpDir.
	Dump(dumpDir string) error
}

var getMongodumpPath = func() (string, error) {
	mongod, err := mongo.Path()
	if err != nil {
		return "", errors.Annotate(err, "failed to get mongod path")
	}
	mongoDumpPath := filepath.Join(filepath.Dir(mongod), dumpName)

	if _, err := os.Stat(mongoDumpPath); err == nil {
		// It already exists so no need to continue.
		return mongoDumpPath, nil
	}

	path, err := exec.LookPath(dumpName)
	if err != nil {
		return "", errors.Trace(err)
	}
	return path, nil
}

type mongoDumper struct {
	DBInfo
	// binPath is the path to the dump executable.
	binPath string
}

// NewDBDumper returns a new value with a Dump method for dumping the
// juju state database.
func NewDBDumper(info DBInfo) (DBDumper, error) {
	err := info.Validate()
	if err != nil {
		return nil, errors.Trace(err)
	}

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
	if err := runCommand(md.binPath, options...); err != nil {
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
	err = stripIgnored(ignored, baseDumpDir)
	return errors.Trace(err)
}

// stripIgnored removes the ignored DBs from the mongo dump files.
// This involves deleting DB-specific directories.
func stripIgnored(ignored set.Strings, dumpDir string) error {
	for _, dbName := range ignored.Values() {
		if dbName != "backups" {
			// We allow all ignored databases except "backups" to be
			// included in the archive file.  Restore will be
			// responsible for deleting those databases after
			// restoring them.
			continue
		}
		dirname := filepath.Join(dumpDir, dbName)
		if err := os.RemoveAll(dirname); err != nil {
			return errors.Trace(err)
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
