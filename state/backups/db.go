// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"

	"github.com/juju/juju/internal/mongo"
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
	// ApproxSizeMB is the storage needed to back up the database.
	ApproxSizeMB int
}

// ignoredDatabases is the list of databases that should not be
// backed up, admin might be removed later, after determining
// mongo version.
var ignoredDatabases = set.NewStrings(
	"admin",
	"backups",
	"presence", // note: this is still backed up anyway
)

// DBSession is a subset of mgo.Session.
type DBSession interface {
	DatabaseNames() ([]string, error)
	DB(name string) Database
}

// Database is a subset of mgo.Database.
type Database interface {
	Run(cmd interface{}, result interface{}) error
}

// NewDBInfo returns the information needed by backups to dump
// the database.
func NewDBInfo(mgoInfo *mongo.MongoInfo, session DBSession) (*DBInfo, error) {
	targets, err := getBackupTargetDatabases(session)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var totalDataSize float64
	for _, target := range targets.Values() {
		var result bson.M
		err := session.DB(target).Run(bson.D{
			{"dbStats", 1},
			{"scale", humanize.MiByte},
		}, &result)
		if err != nil {
			return nil, errors.Trace(err)
		}
		dataSize, ok := result["dataSize"].(float64)
		if !ok {
			return nil, errors.Errorf("missing 'dataSize' value in db stats for database %q", target)
		}
		logger.Debugf("dataSize for %q is %dMiB", target, dataSize)
		totalDataSize += dataSize
	}

	info := DBInfo{
		Address:      mgoInfo.Addrs[0],
		Password:     mgoInfo.Password,
		Targets:      targets,
		ApproxSizeMB: int(totalDataSize),
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
	dumpName       = "mongodump"
	snapToolPrefix = "juju-db."
	snapTmpDir     = "/tmp/snap-private-tmp/snap.juju-db"
)

// DBDumper is any type that dumps something to a dump dir.
type DBDumper interface {
	// Dump something to dumpDir.
	Dump(dumpDir string) error

	// IsSnap returns true if we are using the juju-db snap.
	IsSnap() bool
}

var getMongodumpPath = func() (string, error) {
	return getMongoToolPath(dumpName, os.Stat, exec.LookPath)
}

var getMongodPath = func() (string, error) {
	finder := mongo.NewMongodFinder()
	path, err := finder.InstalledAt()
	return path, err
}

func getMongoToolPath(toolName string, stat func(name string) (os.FileInfo, error), lookPath func(file string) (string, error)) (string, error) {
	mongod, err := getMongodPath()
	if err != nil {
		return "", errors.Annotate(err, "failed to get mongod path")
	}
	mongodDir := filepath.Dir(mongod)

	// Try "juju-db.tool" (how it's named in the Snap).
	mongoTool := filepath.Join(mongodDir, snapToolPrefix+toolName)
	if _, err := stat(mongoTool); err == nil {
		return mongoTool, nil
	}
	logger.Tracef("didn't find MongoDB tool %q in %q", snapToolPrefix+toolName, mongodDir)

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
		"--tlsInsecure",
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
	// Works around https://bugs.launchpad.net/snapd/+bug/1999109
	// If running the juju-db.mongodump snap and staging to /tmp,
	// it outputs to /tmp/snap-private-tmp/snap.juju-db/<dumpDir>
	dumpDirArg := dumpDir
	if md.IsSnap() && strings.HasPrefix(dumpDirArg, snapTmpDir) {
		dumpDirArg = strings.TrimPrefix(dumpDirArg, snapTmpDir)
	}

	options := md.options(dumpDirArg)
	if err := runCommandFn(md.binPath, options...); err != nil {
		return errors.Annotate(err, "error dumping databases")
	}
	return nil
}

// IsSnap returns true if we are using the juju-db snap.
func (md *mongoDumper) IsSnap() bool {
	return filepath.Base(md.binPath) == snapToolPrefix+dumpName
}

// Dump dumps the juju state-related databases.  To do this we dump all
// databases and then remove any ignored databases from the dump results.
func (md *mongoDumper) Dump(baseDumpDir string) error {
	logger.Tracef("dumping Mongo database to %q", baseDumpDir)
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
		case "backups", "admin":
			dirname := filepath.Join(dumpDir, dbName)
			logger.Tracef("stripIgnored deleting dir %q", dirname)
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
	dirEntries, err := os.ReadDir(dumpDir)
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger.Tracef("%d files found in dump dir", len(dirEntries))
	for _, entry := range dirEntries {
		fi, err := entry.Info()
		if err != nil {
			logger.Errorf("failed to read file info: %s", entry.Name())
			continue
		}

		logger.Tracef("file found in dump dir: %q dir=%v size=%d",
			fi.Name(), fi.IsDir(), fi.Size())
	}
	if len(dirEntries) < 2 {
		// Should be *at least* oplog.bson and a data directory
		return nil, errors.Errorf("too few files in dump dir %s (%d)", dumpDir, len(dirEntries))
	}

	databases := make(set.Strings)
	for _, entry := range dirEntries {
		if !entry.IsDir() {
			// Notably, oplog.bson is thus excluded here.
			continue
		}
		databases.Add(entry.Name())
	}
	return databases, nil
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
