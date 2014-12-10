// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"path"
	"time"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/filestorage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/version"
)

// backupIDTimstamp is used to format the timestamp from a backup
// metadata into a string. The result is used as the first half of the
// corresponding backup ID.
const backupIDTimestamp = "20060102-150405"

//---------------------------
// Backup metadata document

// storageMetaDoc is a mirror of backups.Metadata, used just for DB storage.
type storageMetaDoc struct {
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

func (doc *storageMetaDoc) isFileInfoComplete() bool {
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

func (doc *storageMetaDoc) validate() error {
	if doc.ID == "" {
		return errors.New("missing ID")
	}
	if doc.Started == 0 {
		return errors.New("missing Started")
	}
	// We don't check doc.Finished because it doesn't have to be set.
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

	// We don't check doc.Stored because it doesn't have to be set.

	// Check the file-related fields.
	if !doc.isFileInfoComplete() {
		// Don't check the file-related fields.
		return nil
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

func metadocUnixToTime(t int64) time.Time {
	return time.Unix(t, 0).UTC()
}

func metadocTimeToUnix(t time.Time) int64 {
	return t.UTC().Unix()
}

// docAsMetadata returns a new backups.Metadata based on the storageMetaDoc.
func docAsMetadata(doc *storageMetaDoc) *Metadata {
	meta := NewMetadata()
	meta.Started = metadocUnixToTime(doc.Started)
	meta.Notes = doc.Notes

	meta.Origin.Environment = doc.Environment
	meta.Origin.Machine = doc.Machine
	meta.Origin.Hostname = doc.Hostname
	meta.Origin.Version = doc.Version

	meta.SetID(doc.ID)

	if doc.Finished != 0 {
		finished := metadocUnixToTime(doc.Finished)
		meta.Finished = &finished
	}

	if doc.isFileInfoComplete() {
		// Set the file-related fields.

		// The doc should have already been validated when stored.
		meta.FileMetadata.Raw.Size = doc.Size
		meta.FileMetadata.Raw.Checksum = doc.Checksum
		meta.FileMetadata.Raw.ChecksumFormat = doc.ChecksumFormat
	}

	if doc.Stored != 0 {
		stored := metadocUnixToTime(doc.Stored)
		meta.SetStored(&stored)
	}

	return meta
}

// newStorageMetaDoc creates a storageMetaDoc using the corresponding
// values from the backup Metadata.
func newStorageMetaDoc(meta *Metadata) storageMetaDoc {
	var doc storageMetaDoc

	// Ignore metadata.ID. It will be set by storage later.

	doc.Checksum = meta.Checksum()
	doc.ChecksumFormat = meta.ChecksumFormat()
	doc.Size = meta.Size()
	if meta.Stored() != nil {
		stored := meta.Stored()
		doc.Stored = metadocTimeToUnix(*stored)
	}

	doc.Started = metadocTimeToUnix(meta.Started)
	if meta.Finished != nil {
		doc.Finished = metadocTimeToUnix(*meta.Finished)
	}
	doc.Notes = meta.Notes

	doc.Environment = meta.Origin.Environment
	doc.Machine = meta.Origin.Machine
	doc.Hostname = meta.Origin.Hostname
	doc.Version = meta.Origin.Version

	return doc
}

//---------------------------
// DB operations

// TODO(ericsnow) Merge storageDBWrapper with the storage implementation in
// state/storage.go (also see state/toolstorage).

// storageDBWrapper performs all state database operations needed for backups.
type storageDBWrapper struct {
	session   *mgo.Session
	db        *mgo.Database
	metaColl  *mgo.Collection
	txnRunner jujutxn.Runner
	envUUID   string
}

// newStorageDBWrapper returns a DB operator for the , with its own session.
func newStorageDBWrapper(db *mgo.Database, metaColl, envUUID string) *storageDBWrapper {
	session := db.Session.Copy()
	db = db.With(session)

	coll := db.C(metaColl)
	txnRunner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
	dbWrap := storageDBWrapper{
		session:   session,
		db:        db,
		metaColl:  coll,
		txnRunner: txnRunner,
		envUUID:   envUUID,
	}
	return &dbWrap
}

// metadata populates doc with the document matching the ID.
func (b *storageDBWrapper) metadata(id string, doc interface{}) error {
	err := b.metaColl.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("backup metadata %q", id)
	}
	return errors.Trace(err)
}

// allMetadata populates docs with the list of documents in storage.
func (b *storageDBWrapper) allMetadata(docs interface{}) error {
	err := b.metaColl.Find(nil).All(docs)
	return errors.Trace(err)
}

// removeMetadataID removes the identified metadata from storage.
func (b *storageDBWrapper) removeMetadataID(id string) error {
	err := b.metaColl.RemoveId(id)
	return errors.Trace(err)
}

// txnOp returns a single transaction operation populated with the id
// and the metadata collection name. The caller should set other op
// values as needed.
func (b *storageDBWrapper) txnOpBase(id string) txn.Op {
	op := txn.Op{
		C:  b.metaColl.Name,
		Id: id,
	}
	return op
}

// txnOpInsert returns a single transaction operation that will insert
// the document into storage.
func (b *storageDBWrapper) txnOpInsert(id string, doc interface{}) txn.Op {
	op := b.txnOpBase(id)
	op.Assert = txn.DocMissing
	op.Insert = doc
	return op
}

// txnOpInsert returns a single transaction operation that will update
// the already stored document.
func (b *storageDBWrapper) txnOpUpdate(id string, updates ...bson.DocElem) txn.Op {
	op := b.txnOpBase(id)
	op.Assert = txn.DocExists
	op.Update = bson.D{{"$set", bson.D(updates)}}
	return op
}

// runTransaction runs the DB operations within a single transaction.
func (b *storageDBWrapper) runTransaction(ops []txn.Op) error {
	err := b.txnRunner.RunTransaction(ops)
	return errors.Trace(err)
}

// blobStorage returns a ManagedStorage matching the env storage and the blobDB.
func (b *storageDBWrapper) blobStorage(blobDB string) blobstore.ManagedStorage {
	dataStore := blobstore.NewGridFS(blobDB, b.envUUID, b.session)
	return blobstore.NewManagedStorage(b.db, dataStore)
}

// Copy returns a copy of the operator.
func (b *storageDBWrapper) Copy() *storageDBWrapper {
	session := b.session.Copy()

	coll := b.metaColl.With(session)
	db := coll.Database
	txnRunner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
	dbWrap := storageDBWrapper{
		session:   session,
		db:        db,
		metaColl:  coll,
		txnRunner: txnRunner,
		envUUID:   b.envUUID,
	}
	return &dbWrap
}

// Close releases the DB connection resources.
func (b *storageDBWrapper) Close() error {
	b.session.Close()
	return nil
}

// getStorageMetadata returns the backup metadata associated with "id".
// If "id" does not match any stored records, an error satisfying
// juju/errors.IsNotFound() is returned.
func getStorageMetadata(dbWrap *storageDBWrapper, id string) (*storageMetaDoc, error) {
	var doc storageMetaDoc
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

// newStorageID returns a new ID for a state backup.  The format is the
// UTC timestamp from the metadata followed by the environment ID:
// "YYYYMMDD-hhmmss.<env ID>".  This makes the ID a little more human-
// consumable (in contrast to a plain UUID string).  Ideally we would
// use some form of environment name rather than the UUID, but for now
// the raw env ID is sufficient.
var newStorageID = func(doc *storageMetaDoc) string {
	started := metadocUnixToTime(doc.Started)
	timestamp := started.Format(backupIDTimestamp)
	return timestamp + "." + doc.Environment
}

// addStorageMetadata stores metadata for a backup where it can be
// accessed later. It returns a new ID that is associated with the
// backup. If the provided metadata already has an ID set, it is
// ignored. The new ID is set on the doc, even when there is an error.
func addStorageMetadata(dbWrap *storageDBWrapper, doc *storageMetaDoc) (string, error) {
	// We use our own mongo _id value since the auto-generated one from
	// mongo may contain sensitive data (see bson.ObjectID).
	id := newStorageID(doc)

	doc.ID = id
	if err := doc.validate(); err != nil {
		return "", errors.Trace(err)
	}

	op := dbWrap.txnOpInsert(id, doc)

	if err := dbWrap.runTransaction([]txn.Op{op}); err != nil {
		if errors.Cause(err) == txn.ErrAborted {
			return "", errors.AlreadyExistsf("backup metadata %q", doc.ID)
		}
		return "", errors.Annotate(err, "while running transaction")
	}

	return id, nil
}

// setStorageStoredTime updates the backup metadata associated with "id"
// to indicate that a backup archive has been stored.  If "id" does
// not match any stored records, an error satisfying
// juju/errors.IsNotFound() is returned.
func setStorageStoredTime(dbWrap *storageDBWrapper, id string, stored time.Time) error {
	op := dbWrap.txnOpUpdate(id, bson.DocElem{"stored", metadocTimeToUnix(stored)})
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
	dbWrap *storageDBWrapper
}

type backupsMetadataStorage struct {
	filestorage.MetadataDocStorage
	db      *mgo.Database
	envUUID string
}

func newMetadataStorage(dbWrap *storageDBWrapper) *backupsMetadataStorage {
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
	metadata, ok := doc.(*Metadata)
	if !ok {
		return "", errors.Errorf("doc must be of type *backups.Metadata")
	}
	metaDoc := newStorageMetaDoc(metadata)

	dbWrap := s.dbWrap.Copy()
	defer dbWrap.Close()

	id, err := addStorageMetadata(dbWrap, &metaDoc)
	return id, errors.Trace(err)
}

// Doc returns the stored document associated with the given ID.
func (s *backupsDocStorage) Doc(id string) (filestorage.Document, error) {
	dbWrap := s.dbWrap.Copy()
	defer dbWrap.Close()

	doc, err := getStorageMetadata(dbWrap, id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	metadata := docAsMetadata(doc)
	return metadata, nil
}

// ListDocs returns the list of all stored documents.
func (s *backupsDocStorage) ListDocs() ([]filestorage.Document, error) {
	dbWrap := s.dbWrap.Copy()
	defer dbWrap.Close()

	var docs []storageMetaDoc
	if err := dbWrap.allMetadata(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	list := make([]filestorage.Document, len(docs))
	for i, doc := range docs {
		meta := docAsMetadata(&doc)
		list[i] = meta
	}
	return list, nil
}

// RemoveDoc removes the identified document from storage.
func (s *backupsDocStorage) RemoveDoc(id string) error {
	dbWrap := s.dbWrap.Copy()
	defer dbWrap.Close()

	return errors.Trace(dbWrap.removeMetadataID(id))
}

// Close releases the DB resources.
func (s *backupsDocStorage) Close() error {
	return s.dbWrap.Close()
}

// SetStored records in the metadata the fact that the file was stored.
func (s *backupsMetadataStorage) SetStored(id string) error {
	dbWrap := newStorageDBWrapper(s.db, storageMetaName, s.envUUID)
	defer dbWrap.Close()

	err := setStorageStoredTime(dbWrap, id, time.Now())
	return errors.Trace(err)
}

//---------------------------
// raw file storage

const backupStorageRoot = "backups"

type backupBlobStorage struct {
	dbWrap *storageDBWrapper

	envUUID   string
	storeImpl blobstore.ManagedStorage
	root      string
}

func newFileStorage(dbWrap *storageDBWrapper, root string) filestorage.RawFileStorage {
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
	return file, errors.Trace(err)
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

const (
	storageDBName   = "backups"
	storageMetaName = "metadata"
)

// DB represents the set of methods required to perform a backup.
// It exists to break the strict dependency between state and this package,
// and those that depend on this package.
type DB interface {
	// MongoSession returns the underlying mongodb session.
	MongoSession() *mgo.Session

	// EnvironTag is the concrete environ tag for this database.
	EnvironTag() names.EnvironTag
}

// NewStorage returns a new FileStorage to use for storing backup
// archives (and metadata).
func NewStorage(st DB) filestorage.FileStorage {
	envUUID := st.EnvironTag().Id()
	db := st.MongoSession().DB(storageDBName)
	dbWrap := newStorageDBWrapper(db, storageMetaName, envUUID)
	defer dbWrap.Close()

	files := newFileStorage(dbWrap, backupStorageRoot)
	docs := newMetadataStorage(dbWrap)
	return filestorage.NewFileStorage(docs, files)
}
