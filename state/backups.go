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
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/backups"
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

//---------------------------
// Backup metadata document

// backupMetaDoc is a mirror of backups.Metadata, used just for DB storage.
type backupMetaDoc struct {
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

func (doc *backupMetaDoc) fileSet() bool {
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

func (doc *backupMetaDoc) validate() error {
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

// asMetadata returns a new backups.Metadata based on the backupMetaDoc.
func (doc *backupMetaDoc) asMetadata() *backups.Metadata {
	meta := backups.Metadata{
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
// Metadata into the backupMetaDoc.
func (doc *backupMetaDoc) UpdateFromMetadata(meta *backups.Metadata) {
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

// TODO(ericsnow) Merge backupDBWrapper with the storage implementation in
// state/storage.go (also see state/toolstorage).

// backupDBWrapper performs all state database operations needed for
// backups.
type backupDBWrapper struct {
	session   *mgo.Session
	db        *mgo.Database
	metaColl  *mgo.Collection
	txnRunner jujutxn.Runner
	envUUID   string
}

// newBackupDBWrapper returns a DB operator for the , with its own session.
func newBackupDBWrapper(db *mgo.Database, metaColl, envUUID string) *backupDBWrapper {
	session := db.Session.Copy()
	db = db.With(session)

	coll := db.C(metaColl)
	txnRunner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
	dbWrap := backupDBWrapper{
		session:   session,
		db:        db,
		metaColl:  coll,
		txnRunner: txnRunner,
		envUUID:   envUUID,
	}
	return &dbWrap
}

// metadata populates doc with the document matching the ID.
func (b *backupDBWrapper) metadata(id string, doc interface{}) error {
	err := b.metaColl.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("backup metadata %q", id)
	}
	return errors.Trace(err)
}

// allMetadata populates docs with the list of documents in storage.
func (b *backupDBWrapper) allMetadata(docs interface{}) error {
	err := b.metaColl.Find(nil).All(docs)
	return errors.Trace(err)
}

// removeMetadata removes the identified metadata from storage.
func (b *backupDBWrapper) removeMetadata(id string) error {
	err := b.metaColl.RemoveId(id)
	return errors.Trace(err)
}

// txnOp returns a single transaction operation populated with the id
// and the metadata collection name.
func (b *backupDBWrapper) txnOp(id string) txn.Op {
	op := txn.Op{
		C:  b.metaColl.Name,
		Id: id,
	}
	return op
}

// runTransaction runs the DB operations within a single transaction.
func (b *backupDBWrapper) runTransaction(ops []txn.Op) error {
	err := b.txnRunner.RunTransaction(ops)
	return errors.Trace(err)
}

// blobStorage returns a ManagedStorage matching the env storage and
// the blobDB.
func (b *backupDBWrapper) blobStorage(blobDB string) blobstore.ManagedStorage {
	dataStore := blobstore.NewGridFS(blobDB, b.envUUID, b.session)
	return blobstore.NewManagedStorage(b.db, dataStore)
}

// Copy returns a copy of the operator.
func (b *backupDBWrapper) Copy() *backupDBWrapper {
	session := b.session.Copy()

	coll := b.metaColl.With(session)
	db := coll.Database
	txnRunner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
	dbWrap := backupDBWrapper{
		session:   session,
		db:        db,
		metaColl:  coll,
		txnRunner: txnRunner,
		envUUID:   b.envUUID,
	}
	return &dbWrap
}

// Close releases the DB connection resources.
func (b *backupDBWrapper) Close() error {
	b.session.Close()
	return nil
}

// getBackupMetadata returns the backup metadata associated with "id".
// If "id" does not match any stored records, an error satisfying
// juju/errors.IsNotFound() is returned.
func getBackupMetadata(dbWrap *backupDBWrapper, id string) (*backupMetaDoc, error) {
	var doc backupMetaDoc
	// There can only be one!
	err := dbWrap.metadata(id, &doc)
	if errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	} else if err != nil {
		return nil, errors.Annotate(err, "while getting metadata")
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
func newBackupID(doc *backupMetaDoc) string {
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
func addBackupMetadata(dbWrap *backupDBWrapper, doc *backupMetaDoc) (string, error) {
	// We use our own mongo _id value since the auto-generated one from
	// mongo may contain sensitive data (see bson.ObjectID).
	id := newBackupID(doc)
	return id, addBackupMetadataID(dbWrap, doc, id)
}

func addBackupMetadataID(dbWrap *backupDBWrapper, doc *backupMetaDoc, id string) error {
	doc.ID = id
	if err := doc.validate(); err != nil {
		return errors.Trace(err)
	}

	op := dbWrap.txnOp(id)
	op.Assert = txn.DocMissing
	op.Insert = doc

	if err := dbWrap.runTransaction([]txn.Op{op}); err != nil {
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
func setBackupStored(dbWrap *backupDBWrapper, id string, stored time.Time) error {
	op := dbWrap.txnOp(id)
	op.Assert = txn.DocExists
	op.Update = bson.D{{"$set", bson.D{
		{"stored", stored.UTC().Unix()},
	}}}

	if err := dbWrap.runTransaction([]txn.Op{op}); err != nil {
		if errors.Cause(err) == txn.ErrAborted {
			return errors.NotFoundf("backup metadata %q", id)
		}
		return errors.Annotate(err, "while running transaction")
	}
	return nil
}

//---------------------------
// metadata storage

type backupsDocStorage struct {
	dbWrap *backupDBWrapper
}

type backupsMetadataStorage struct {
	filestorage.MetadataDocStorage
	db      *mgo.Database
	envUUID string
}

func newBackupMetadataStorage(dbWrap *backupDBWrapper) *backupsMetadataStorage {
	dbWrap = dbWrap.Copy()

	docStor := backupsDocStorage{dbWrap}
	stor := backupsMetadataStorage{
		MetadataDocStorage: filestorage.MetadataDocStorage{&docStor},
		db:                 dbWrap.db,
		envUUID:            dbWrap.envUUID,
	}
	return &stor
}

// AddDoc adds the document to storage and returns the new ID.
func (s *backupsDocStorage) AddDoc(doc filestorage.Document) (string, error) {
	var metaDoc backupMetaDoc
	metadata, ok := doc.(*backups.Metadata)
	if !ok {
		return "", errors.Errorf("doc must be of type *backups.Metadata")
	}
	metaDoc.UpdateFromMetadata(metadata)

	dbWrap := s.dbWrap.Copy()
	defer dbWrap.Close()

	id, err := addBackupMetadata(dbWrap, &metaDoc)
	return id, errors.Trace(err)
}

// Doc returns the stored document associated with the given ID.
func (s *backupsDocStorage) Doc(id string) (filestorage.Document, error) {
	dbWrap := s.dbWrap.Copy()
	defer dbWrap.Close()

	doc, err := getBackupMetadata(dbWrap, id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	metadata := doc.asMetadata()
	return metadata, nil
}

// ListDocs returns the list of all stored documents.
func (s *backupsDocStorage) ListDocs() ([]filestorage.Document, error) {
	dbWrap := s.dbWrap.Copy()
	defer dbWrap.Close()

	var docs []backupMetaDoc
	if err := dbWrap.allMetadata(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	list := make([]filestorage.Document, len(docs))
	for i, doc := range docs {
		meta := doc.asMetadata()
		list[i] = meta
	}
	return list, nil
}

// RemoveDoc removes the identified document from storage.
func (s *backupsDocStorage) RemoveDoc(id string) error {
	dbWrap := s.dbWrap.Copy()
	defer dbWrap.Close()

	return errors.Trace(dbWrap.removeMetadata(id))
}

// Close releases the DB resources.
func (s *backupsDocStorage) Close() error {
	return s.dbWrap.Close()
}

// SetStored records in the metadata the fact that the file was stored.
func (s *backupsMetadataStorage) SetStored(id string) error {
	dbWrap := newBackupDBWrapper(s.db, backupsMetaC, s.envUUID)
	defer dbWrap.Close()

	err := setBackupStored(dbWrap, id, time.Now())
	return errors.Trace(err)
}

//---------------------------
// raw file storage

const backupStorageRoot = "backups"

type backupBlobStorage struct {
	dbWrap *backupDBWrapper

	envUUID   string
	storeImpl blobstore.ManagedStorage
	root      string
}

func newBackupFileStorage(dbWrap *backupDBWrapper, root string) filestorage.RawFileStorage {
	dbWrap = dbWrap.Copy()

	managed := dbWrap.blobStorage(dbWrap.db.Name)
	stor := backupBlobStorage{
		dbWrap:    dbWrap,
		envUUID:   dbWrap.envUUID,
		storeImpl: managed,
		root:      root,
	}
	return &stor
}

func (s *backupBlobStorage) path(id string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join(s.root, id)
}

// File returns the identified file from storage.
func (s *backupBlobStorage) File(id string) (io.ReadCloser, error) {
	file, _, err := s.storeImpl.GetForEnvironment(s.envUUID, s.path(id))
	return file, err
}

// AddFile adds the file to storage.
func (s *backupBlobStorage) AddFile(id string, file io.Reader, size int64) error {
	return s.storeImpl.PutForEnvironment(s.envUUID, s.path(id), file, size)
}

// RemoveFile removes the identified file from storage.
func (s *backupBlobStorage) RemoveFile(id string) error {
	return s.storeImpl.RemoveForEnvironment(s.envUUID, s.path(id))
}

// Close closes the storage.
func (s *backupBlobStorage) Close() error {
	return s.dbWrap.Close()
}

//---------------------------
// backup storage

const backupDB = "backups"

// NewBackupStorage returns a new FileStorage to use for storing backup
// archives (and metadata).
func NewBackupStorage(st *State) filestorage.FileStorage {
	envUUID := st.EnvironTag().Id()
	db := st.MongoSession().DB(backupDB)
	dbWrap := newBackupDBWrapper(db, backupsMetaC, envUUID)
	defer dbWrap.Close()

	files := newBackupFileStorage(dbWrap, backupStorageRoot)
	docs := newBackupMetadataStorage(dbWrap)
	return filestorage.NewFileStorage(docs, files)
}

// NewBackups returns a new backups based on the state.
func NewBackups(st *State) (backups.Backups, io.Closer) {
	stor := NewBackupStorage(st)

	backups := backups.NewBackups(stor)
	return backups, stor
}

//---------------------------
// utilities

// ignoredDatabases is the list of databases that should not be
// backed up.
var ignoredDatabases = set.NewStrings(
	"backups",
	"presence",
)

// NewDBBackupInfo returns the information needed by backups to dump
// the database.
func NewDBBackupInfo(st *State) (*backups.DBInfo, error) {
	connInfo := newMongoConnInfo(st.MongoConnectionInfo())
	targets, err := getBackupTargetDatabases(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info := backups.DBInfo{
		DBConnInfo: *connInfo,
		Targets:    targets,
	}
	return &info, nil
}

func newMongoConnInfo(mgoInfo *mongo.MongoInfo) *backups.DBConnInfo {
	info := backups.DBConnInfo{
		Address:  mgoInfo.Addrs[0],
		Password: mgoInfo.Password,
	}

	// TODO(dfc) Backup should take a Tag.
	if mgoInfo.Tag != nil {
		info.Username = mgoInfo.Tag.String()
	}

	return &info
}

func getBackupTargetDatabases(st *State) (set.Strings, error) {
	dbNames, err := st.MongoSession().DatabaseNames()
	if err != nil {
		return nil, errors.Annotate(err, "unable to get DB names")
	}

	targets := set.NewStrings(dbNames...).Difference(ignoredDatabases)
	return targets, nil
}

// NewBackupOrigin returns a snapshot of where backup was run.  That
// snapshot is a new backup Origin value, for use in a backup's
// metadata.  Every value except for the machine name is populated
// either from juju state or some other implicit mechanism.
func NewBackupOrigin(st *State, machine string) (*backups.Origin, error) {
	// hostname could be derived from the environment...
	hostname, err := os.Hostname()
	if err != nil {
		// If os.Hostname() is not working, something is woefully wrong.
		// Run for the hills.
		return nil, errors.Annotate(err, "could not get hostname (system unstable?)")
	}
	origin := backups.NewOrigin(
		st.EnvironTag().Id(),
		machine,
		hostname,
	)
	return &origin, nil
}
