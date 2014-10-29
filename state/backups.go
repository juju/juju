// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/filestorage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/version"
)

/*
Backups are not a part of juju state nor of normal state operations.
However, they certainly are tightly coupled with state (the very
subject of backups).  This puts backups in an odd position,
particularly with regard to the storage of backup metadata and
archives.  As a result, here are a couple concerns worth mentioning.

First, as noted above backup is about state but not a part of state.
So exposing backup-related methods on State would imply the wrong
thing.  Thus the backup functionality here in the state package (not
state/backups) is exposed as functions to which you pass a state
object.

Second, backup creates an archive file containing a dump of state's
mongo DB.  Storing backup metadata/archives in mongo is thus a
somewhat circular proposition that has the potential to cause
problems.  That may need further attention.

Note that state (and juju as a whole) currently does not have a
persistence layer abstraction to facilitate separating different
persistence needs and implementations.  As a consequence, state's
data, whether about how an environment should look or about existing
resources within an environment, is dumped essentially straight into
State's mongo connection.  The code in the state package does not
make any distinction between the two (nor does the package clearly
distinguish between state-related abstractions and state-related
data).

Backup adds yet another category, merely taking advantage of
State's DB.  In the interest of making the distinction clear, the
code that directly interacts with State (and its DB) lives in this
file.  As mentioned previously, the functionality here is exposed
through functions that take State, rather than as methods on State.
Furthermore, the bulk of the backup-related code, which does not need
direct interaction with State, lives in the state/backups package.
*/

// BackupMetaDoc is a mirror of metadata.Metadata, used just for DB storage.
type BackupMetaDoc struct {
	ID string `bson:"_id"`

	// blob storage

	Checksum       string `bson:"checksum"`
	ChecksumFormat string `bson:"checksumformat"`
	Size           int64  `bson:"size,minsize"`
	Stored         int64  `bson:"stored,minsize"`

	// backup

	Started  int64  `bson:"started,minsize"`
	Finished int64  `bson:"finished,minsize"`
	Notes    string `bson:"notes,omitempty"`

	// origin

	Environment string         `bson:"environment"`
	Machine     string         `bson:"machine"`
	Hostname    string         `bson:"hostname"`
	Version     version.Number `bson:"version"`
}

func (doc *BackupMetaDoc) fileSet() bool {
	if doc.Finished == 0 {
		return false
	}
	if doc.Checksum == "" {
		return false
	}
	if doc.ChecksumFormat == "" {
		return false
	}
	if doc.Size == 0 {
		return false
	}
	return true
}

func (doc *BackupMetaDoc) validate() error {
	if doc.ID == "" {
		return errors.New("missing ID")
	}
	if doc.Started == 0 {
		return errors.New("missing Started")
	}
	if doc.Environment == "" {
		return errors.New("missing Environment")
	}
	if doc.Machine == "" {
		return errors.New("missing Machine")
	}
	if doc.Hostname == "" {
		return errors.New("missing Hostname")
	}
	if doc.Version.Major == 0 {
		return errors.New("missing Version")
	}

	// Check the file-related fields.
	if !doc.fileSet() {
		if doc.Stored != 0 {
			return errors.New(`"Stored" flag is unexpectedly true`)
		}
		// Don't check the file-related fields.
		return nil
	}
	if doc.Finished == 0 {
		return errors.New("missing Finished")
	}
	if doc.Checksum == "" {
		return errors.New("missing Checksum")
	}
	if doc.ChecksumFormat == "" {
		return errors.New("missing ChecksumFormat")
	}
	if doc.Size == 0 {
		return errors.New("missing Size")
	}

	return nil
}

// asMetadata returns a new metadata.Metadata based on the BackupMetaDoc.
func (doc *BackupMetaDoc) asMetadata() *metadata.Metadata {
	meta := metadata.Metadata{
		Started: time.Unix(doc.Started, 0).UTC(),
		Notes:   doc.Notes,
	}

	meta.Origin.Environment = doc.Environment
	meta.Origin.Machine = doc.Machine
	meta.Origin.Hostname = doc.Hostname
	meta.Origin.Version = doc.Version

	meta.SetID(doc.ID)

	if doc.fileSet() {
		// Set the file-related fields.

		finished := time.Unix(doc.Finished, 0).UTC()
		meta.Finished = &finished

		// The doc should have already been validated when stored.
		meta.FileMetadata.Raw.Size = doc.Size
		meta.FileMetadata.Raw.Checksum = doc.Checksum
		meta.FileMetadata.Raw.ChecksumFormat = doc.ChecksumFormat

		if doc.Stored != 0 {
			stored := time.Unix(doc.Stored, 0).UTC()
			meta.SetStored(&stored)
		}
	}

	return &meta
}

// UpdateFromMetadata copies the corresponding data from the backup
// Metadata into the BackupMetaDoc.
func (doc *BackupMetaDoc) UpdateFromMetadata(meta *metadata.Metadata) {
	// Ignore metadata.ID.

	doc.Checksum = meta.Checksum()
	doc.ChecksumFormat = meta.ChecksumFormat()
	doc.Size = meta.Size()
	if meta.Stored() != nil {
		doc.Stored = meta.Stored().Unix()
	}

	doc.Started = meta.Started.Unix()
	if meta.Finished != nil {
		doc.Finished = meta.Finished.Unix()
	}
	doc.Notes = meta.Notes

	doc.Environment = meta.Origin.Environment
	doc.Machine = meta.Origin.Machine
	doc.Hostname = meta.Origin.Hostname
	doc.Version = meta.Origin.Version
}

//---------------------------
// DB operations

// DBOperator holds the information necessary to perform state database
// operations.
type DBOperator struct {
	session   *mgo.Session
	txnRunner jujutxn.Runner

	// EnvUUID is the UUID of the environment.
	EnvUUID string
	// Target is the DB collection on which to operate.
	Target *mgo.Collection
}

// NewDBOperator returns a DB operator for the target, with its own session.
func NewDBOperator(db *mgo.Database, target, envUUID string) *DBOperator {
	session := db.Session.Copy()
	db = db.With(session)

	coll := db.C(target)
	txnRunner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
	dbOp := DBOperator{
		session:   session,
		txnRunner: txnRunner,
		EnvUUID:   envUUID,
		Target:    coll,
	}
	return &dbOp
}

// RunTransaction runs the DB operations within a single transaction.
func (o *DBOperator) RunTransaction(ops []txn.Op) error {
	err := o.txnRunner.RunTransaction(ops)
	return errors.Trace(err)
}

// Copy returns a copy of the operator.
func (o *DBOperator) Copy() *DBOperator {
	session := o.session.Copy()

	coll := o.Target.With(session)
	db := coll.Database
	txnRunner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
	dbOp := DBOperator{
		session:   session,
		txnRunner: txnRunner,
		EnvUUID:   o.EnvUUID,
		Target:    coll,
	}
	return &dbOp
}

// Close releases the DB connection resources.
func (o *DBOperator) Close() error {
	o.session.Close()
	return nil
}

// getBackupMetadata returns the backup metadata associated with "id".
// If "id" does not match any stored records, an error satisfying
// juju/errors.IsNotFound() is returned.
func getBackupMetadata(dbOp *DBOperator, id string) (*BackupMetaDoc, error) {
	var doc BackupMetaDoc
	// There can only be one!
	err := dbOp.Target.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("backup metadata %q", id)
	} else if err != nil {
		return nil, errors.Annotate(err, "error getting backup metadata")
	}

	if err := doc.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}

// newBackupID returns a new ID for a state backup.  The format is the
// UTC timestamp from the metadata followed by the environment ID:
// "YYYYMMDD-hhmmss.<env ID>".  This makes the ID a little more human-
// consumable (in contrast to a plain UUID string).  Ideally we would
// use some form of environment name rather than the UUID, but for now
// the raw env ID is sufficient.
func newBackupID(doc *BackupMetaDoc) string {
	rawts := time.Unix(doc.Started, 0).UTC()
	Y, M, D := rawts.Date()
	h, m, s := rawts.Clock()
	timestamp := fmt.Sprintf("%04d%02d%02d-%02d%02d%02d", Y, M, D, h, m, s)

	return timestamp + "." + doc.Environment
}

// addBackupMetadata stores metadata for a backup where it can be
// accessed later.  It returns a new ID that is associated with the
// backup.  If the provided metadata already has an ID set, it is
// ignored.
func addBackupMetadata(dbOp *DBOperator, doc *BackupMetaDoc) (string, error) {
	// We use our own mongo _id value since the auto-generated one from
	// mongo may contain sensitive data (see bson.ObjectID).
	id := newBackupID(doc)
	return id, addBackupMetadataID(dbOp, doc, id)
}

func addBackupMetadataID(dbOp *DBOperator, doc *BackupMetaDoc, id string) error {
	doc.ID = id
	if err := doc.validate(); err != nil {
		return errors.Trace(err)
	}

	ops := []txn.Op{{
		C:      dbOp.Target.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: doc,
	}}

	if err := dbOp.RunTransaction(ops); err != nil {
		if errors.Cause(err) == txn.ErrAborted {
			return errors.AlreadyExistsf("backup metadata %q", doc.ID)
		}
		return errors.Annotate(err, "while running transaction")
	}

	return nil
}

// setBackupStored updates the backup metadata associated with "id"
// to indicate that a backup archive has been stored.  If "id" does
// not match any stored records, an error satisfying
// juju/errors.IsNotFound() is returned.
func setBackupStored(dbOp *DBOperator, id string, stored time.Time) error {
	ops := []txn.Op{{
		C:      dbOp.Target.Name,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"stored", stored.UTC().Unix()},
		}}},
	}}

	if err := dbOp.RunTransaction(ops); err != nil {
		if errors.Cause(err) == txn.ErrAborted {
			return errors.NotFoundf(id)
		}
		return errors.Annotate(err, "while running transaction")
	}
	return nil
}

//---------------------------
// metadata storage

// NewBackupsOrigin returns a snapshot of where backup was run.  That
// snapshot is a new backup Origin value, for use in a backup's
// metadata.  Every value except for the machine name is populated
// either from juju state or some other implicit mechanism.
func NewBackupsOrigin(st *State, machine string) *metadata.Origin {
	// hostname could be derived from the environment...
	hostname, err := os.Hostname()
	if err != nil {
		// If os.Hostname() is not working, something is woefully wrong.
		// Run for the hills.
		panic(fmt.Sprintf("could not get hostname (system unstable?): %v", err))
	}
	origin := metadata.NewOrigin(
		st.EnvironTag().Id(),
		machine,
		hostname,
	)
	return origin
}

type backupsDocStorage struct {
	dbOp *DBOperator
}

type backupsMetadataStorage struct {
	filestorage.MetadataDocStorage
	db      *mgo.Database
	envUUID string
}

func newBackupMetadataStorage(dbOp *DBOperator) *backupsMetadataStorage {
	dbOp = dbOp.Copy()
	db := dbOp.Target.Database

	docStor := backupsDocStorage{dbOp}
	stor := backupsMetadataStorage{
		MetadataDocStorage: filestorage.MetadataDocStorage{&docStor},
		db:                 db,
		envUUID:            dbOp.EnvUUID,
	}
	return &stor
}

func (s *backupsDocStorage) AddDoc(doc filestorage.Document) (string, error) {
	var metaDoc BackupMetaDoc
	metadata, ok := doc.(*metadata.Metadata)
	if !ok {
		return "", errors.Errorf("doc must be of type *metadata.Metadata")
	}
	metaDoc.UpdateFromMetadata(metadata)

	dbOp := s.dbOp.Copy()
	defer dbOp.Close()

	id, err := addBackupMetadata(dbOp, &metaDoc)
	return id, errors.Trace(err)
}

func (s *backupsDocStorage) Doc(id string) (filestorage.Document, error) {
	dbOp := s.dbOp.Copy()
	defer dbOp.Close()

	doc, err := getBackupMetadata(dbOp, id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	metadata := doc.asMetadata()
	return metadata, nil
}

func (s *backupsDocStorage) ListDocs() ([]filestorage.Document, error) {
	dbOp := s.dbOp.Copy()
	defer dbOp.Close()

	var docs []BackupMetaDoc
	if err := dbOp.Target.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	list := make([]filestorage.Document, len(docs))
	for i, doc := range docs {
		meta := doc.asMetadata()
		list[i] = meta
	}
	return list, nil
}

func (s *backupsDocStorage) RemoveDoc(id string) error {
	dbOp := s.dbOp.Copy()
	defer dbOp.Close()

	return errors.Trace(dbOp.Target.RemoveId(id))
}

// Close releases the DB resources.
func (s *backupsDocStorage) Close() error {
	return s.dbOp.Close()
}

// SetStored records in the metadata the fact that the file was stored.
func (s *backupsMetadataStorage) SetStored(id string) error {
	dbOp := NewDBOperator(s.db, backupsMetaC, s.envUUID)
	defer dbOp.Close()

	err := setBackupStored(dbOp, id, time.Now())
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

//---------------------------
// raw file storage

const backupStorageRoot = "backups"

type backupBlobStorage struct {
	dbOp *DBOperator

	envUUID string
	managed blobstore.ManagedStorage
	root    string
}

func newBackupFileStorage(dbOp *DBOperator, root string) filestorage.RawFileStorage {
	dbOp = dbOp.Copy()
	db := dbOp.Target.Database
	dbName := blobstoreDB

	dataStore := blobstore.NewGridFS(dbName, dbOp.EnvUUID, db.Session)
	managed := blobstore.NewManagedStorage(db, dataStore)

	stor := backupBlobStorage{
		dbOp:    dbOp,
		envUUID: dbOp.EnvUUID,
		managed: managed,
		root:    root,
	}
	return &stor
}

func (s *backupBlobStorage) path(id string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join(s.root, id)
}

func (s *backupBlobStorage) File(id string) (io.ReadCloser, error) {
	file, _, err := s.managed.GetForEnvironment(s.envUUID, s.path(id))
	return file, err
}

func (s *backupBlobStorage) AddFile(id string, file io.Reader, size int64) error {
	return s.managed.PutForEnvironment(s.envUUID, s.path(id), file, size)
}

func (s *backupBlobStorage) RemoveFile(id string) error {
	return s.managed.RemoveForEnvironment(s.envUUID, s.path(id))
}

// Close closes the storage.
func (s *backupBlobStorage) Close() error {
	return s.dbOp.Close()
}

//---------------------------
// backup storage

const BackupsDB = "juju"

// NewBackupStorage returns a new FileStorage to use for storing backup
// archives (and metadata).
func NewBackupStorage(st *State) filestorage.FileStorage {
	envUUID := st.EnvironTag().Id()
	db := st.db
	dbOp := NewDBOperator(db, backupsMetaC, envUUID)
	defer dbOp.Close()

	files := newBackupFileStorage(dbOp, backupStorageRoot)
	docs := newBackupMetadataStorage(dbOp)
	return filestorage.NewFileStorage(docs, files)
}

// NewBackups returns a new backups based on the state.
func NewBackups(st *State) (backups.Backups, io.Closer) {
	stor := NewBackupStorage(st)

	backups := backups.NewBackups(stor)
	return backups, stor
}
