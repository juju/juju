// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/filestorage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/storage"
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

// backupMetadataDoc is a mirror of metadata.Metadata, used just for DB storage.
type backupMetadataDoc struct {
	ID string `bson:"_id"`

	// blob storage

	Checksum       string `bson:"checksum"`
	ChecksumFormat string `bson:"checksumformat"`
	Size           int64  `bson:"size,minsize"`
	Stored         int64  `bson:"stored,minsize"`

	// backups

	Started  int64  `bson:"started,minsize"`
	Finished int64  `bson:"finished,minsize"`
	Notes    string `bson:"notes,omitempty"`

	// origin

	Environment string         `bson:"environment"`
	Machine     string         `bson:"machine"`
	Hostname    string         `bson:"hostname"`
	Version     version.Number `bson:"version"`
}

func (doc *backupMetadataDoc) fileSet() bool {
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

func (doc *backupMetadataDoc) validate() error {
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

// asMetadata returns a new metadata.Metadata based on the backupMetadataDoc.
func (doc *backupMetadataDoc) asMetadata() *metadata.Metadata {
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

// updateFromMetadata copies the corresponding data from the backup
// Metadata into the backupMetadataDoc.
func (doc *backupMetadataDoc) updateFromMetadata(meta *metadata.Metadata) {
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

// getBackupMetadata returns the backup metadata associated with "id".
// If "id" does not match any stored records, an error satisfying
// juju/errors.IsNotFound() is returned.
func getBackupMetadata(st *State, id string) (*metadata.Metadata, error) {
	collection, closer := st.getCollection(backupsMetaC)
	defer closer()

	var doc backupMetadataDoc
	// There can only be one!
	err := collection.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("backup metadata %q", id)
	} else if err != nil {
		return nil, errors.Annotate(err, "error getting backup metadata")
	}

	if err := doc.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return doc.asMetadata(), nil
}

// newBackupID returns a new ID for a state backup.  The format is the
// UTC timestamp from the metadata followed by the environment ID:
// "YYYYMMDD-hhmmss.<env ID>".  This makes the ID a little more human-
// consumable (in contrast to a plain UUID string).  Ideally we would
// use some form of environment name rather than the UUID, but for now
// the raw env ID is sufficient.
func newBackupID(meta *metadata.Metadata) string {
	Y, M, D := meta.Started.Date()
	h, m, s := meta.Started.Clock()
	timestamp := fmt.Sprintf("%04d%02d%02d-%02d%02d%02d", Y, M, D, h, m, s)

	return timestamp + "." + meta.Origin.Environment
}

// addBackupMetadata stores metadata for a backup where it can be
// accessed later.  It returns a new ID that is associated with the
// backup.  If the provided metadata already has an ID set, it is
// ignored.
func addBackupMetadata(st *State, metadata *metadata.Metadata) (string, error) {
	// We use our own mongo _id value since the auto-generated one from
	// mongo may contain sensitive data (see bson.ObjectID).
	id := newBackupID(metadata)
	return id, addBackupMetadataID(st, metadata, id)
}

func addBackupMetadataID(st *State, metadata *metadata.Metadata, id string) error {
	var doc backupMetadataDoc
	doc.updateFromMetadata(metadata)
	doc.ID = id
	if err := doc.validate(); err != nil {
		return errors.Trace(err)
	}

	ops := []txn.Op{{
		C:      backupsMetaC,
		Id:     doc.ID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
	if err := st.runTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			return errors.AlreadyExistsf("backup metadata %q", doc.ID)
		}
		return errors.Annotate(err, "error running transaction")
	}

	return nil
}

// setBackupStored updates the backup metadata associated with "id"
// to indicate that a backup archive has been stored.  If "id" does
// not match any stored records, an error satisfying
// juju/errors.IsNotFound() is returned.
func setBackupStored(st *State, id string, stored time.Time) error {
	ops := []txn.Op{{
		C:      backupsMetaC,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"stored", stored.Unix()},
		}}},
	}}
	if err := st.runTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			return errors.NotFoundf(id)
		}
		return errors.Annotate(err, "error running transaction")
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
	state *State
}

type backupsMetadataStorage struct {
	filestorage.MetadataDocStorage
	state *State
}

func newBackupMetadataStorage(st *State) filestorage.MetadataStorage {
	docStor := backupsDocStorage{state: st}
	stor := backupsMetadataStorage{
		MetadataDocStorage: filestorage.MetadataDocStorage{&docStor},
		state:              st,
	}
	return &stor
}

func (s *backupsDocStorage) AddDoc(doc filestorage.Document) (string, error) {
	metadata, ok := doc.(*metadata.Metadata)
	if !ok {
		return "", errors.Errorf("doc must be of type *metadata.Metadata")
	}
	return addBackupMetadata(s.state, metadata)
}

func (s *backupsDocStorage) Doc(id string) (filestorage.Document, error) {
	metadata, err := getBackupMetadata(s.state, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return metadata, nil
}

func (s *backupsDocStorage) ListDocs() ([]filestorage.Document, error) {
	collection, closer := s.state.getCollection(backupsMetaC)
	defer closer()

	var docs []backupMetadataDoc
	if err := collection.Find(nil).All(&docs); err != nil {
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
	collection, closer := s.state.getCollection(backupsMetaC)
	defer closer()

	return errors.Trace(collection.RemoveId(id))
}

// Close implements io.Closer.Close.
func (s *backupsDocStorage) Close() error {
	return nil
}

// SetStored records in the metadata the fact that the file was stored.
func (s *backupsMetadataStorage) SetStored(id string) error {
	err := setBackupStored(s.state, id, time.Now().UTC())
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

//---------------------------
// raw file storage

const backupStorageRoot = "backups"

// Ensure we satisfy the interface.
var _ filestorage.DocStorage = (*backupsDocStorage)(nil)

// Ensure we satisfy the interface.
var _ filestorage.RawFileStorage = (*envFileStorage)(nil)

type envFileStorage struct {
	envStor storage.Storage
	root    string
}

func newBackupFileStorage(envStor storage.Storage, root string) filestorage.RawFileStorage {
	// Due to circular imports we cannot simply get the storage from
	// State using environs.GetStorage().
	stor := envFileStorage{
		envStor: envStor,
		root:    root,
	}
	return &stor
}

func (s *envFileStorage) path(id string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join(s.root, id)
}

func (s *envFileStorage) File(id string) (io.ReadCloser, error) {
	return s.envStor.Get(s.path(id))
}

func (s *envFileStorage) AddFile(id string, file io.Reader, size int64) error {
	return s.envStor.Put(s.path(id), file, size)
}

func (s *envFileStorage) RemoveFile(id string) error {
	return s.envStor.Remove(s.path(id))
}

// Close closes the storage.
func (s *envFileStorage) Close() error {
	return nil
}

//---------------------------
// backup storage

// NewBackupsStorage returns a new FileStorage to use for storing backup
// archives (and metadata).
func NewBackupsStorage(st *State, envStor storage.Storage) filestorage.FileStorage {
	files := newBackupFileStorage(envStor, backupStorageRoot)
	docs := newBackupMetadataStorage(st)
	return filestorage.NewFileStorage(docs, files)
}
