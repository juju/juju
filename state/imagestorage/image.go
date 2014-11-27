// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagestorage

import (
	"fmt"
	"io"
	"time"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/mongo"
)

var logger = loggo.GetLogger("juju.state.imagestorage")

const (
	// imagemetadataC is the collection used to store image metadata.
	imagemetadataC = "imagemetadata"

	// ImagesDB is the database used to store image blobs.
	ImagesDB = "osimages"
)

type imageStorage struct {
	envUUID            string
	metadataCollection *mgo.Collection
	blobDb             *mgo.Database
}

var _ Storage = (*imageStorage)(nil)

// NewStorage constructs a new Storage that stores image blobs
// in an "osimages" database. Image metadata is also stored in this
// database in the "imagemetadata" collection.
func NewStorage(
	session *mgo.Session,
	envUUID string,
) Storage {
	blobDb := session.DB(ImagesDB)
	metadataCollection := blobDb.C(imagemetadataC)
	return &imageStorage{
		envUUID,
		metadataCollection,
		blobDb,
	}
}

// Override for testing.
var getManagedStorage = func(session *mgo.Session) blobstore.ManagedStorage {
	rs := blobstore.NewGridFS(ImagesDB, ImagesDB, session)
	db := session.DB(ImagesDB)
	metadataDb := db.With(session)
	return blobstore.NewManagedStorage(metadataDb, rs)
}

func (s *imageStorage) getManagedStorage() blobstore.ManagedStorage {
	return getManagedStorage(s.blobDb.Session.Copy())
}

func (s *imageStorage) txnRunnerWithSession() (jujutxn.Runner, *mgo.Session) {
	db := s.metadataCollection.Database
	session := db.Session.Copy()
	runnerDb := db.With(session)
	return txnRunner(runnerDb), session
}

// Override for testing.
var txnRunner = func(db *mgo.Database) jujutxn.Runner {
	return jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
}

// AddImage is defined on the Storage interface.
func (s *imageStorage) AddImage(r io.Reader, metadata *Metadata) (resultErr error) {
	managedStorage := s.getManagedStorage()
	path := imagePath(metadata.Kind, metadata.Series, metadata.Arch, metadata.SHA256)
	if err := managedStorage.PutForEnvironment(s.envUUID, path, r, metadata.Size); err != nil {
		return errors.Annotate(err, "cannot store image")
	}
	defer func() {
		if resultErr == nil {
			return
		}
		err := managedStorage.RemoveForEnvironment(s.envUUID, path)
		if err != nil {
			logger.Errorf("failed to remove image blob: %v", err)
		}
	}()

	newDoc := imageMetadataDoc{
		Id:      docId(metadata),
		Kind:    metadata.Kind,
		Series:  metadata.Series,
		Arch:    metadata.Arch,
		Size:    metadata.Size,
		SHA256:  metadata.SHA256,
		Path:    path,
		Created: time.Now().Format(time.RFC3339),
	}

	// Add or replace metadata. If replacing, record the
	// existing path so we can remove the blob later.
	var oldPath string
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:  imagemetadataC,
			Id: newDoc.Id,
		}

		// On the first attempt we assume we're adding a new image blob.
		// Subsequent attempts to add image will fetch the existing
		// doc, record the old path, and attempt to update the
		// size, path and hash fields.
		if attempt == 0 {
			op.Assert = txn.DocMissing
			op.Insert = &newDoc
		} else {
			oldDoc, err := s.imageMetadataDoc(metadata.EnvUUID, metadata.Kind, metadata.Series, metadata.Arch)
			if err != nil {
				return nil, err
			}
			oldPath = oldDoc.Path
			op.Assert = bson.D{{"path", oldPath}}
			if oldPath != path {
				op.Update = bson.D{{
					"$set", bson.D{
						{"size", metadata.Size},
						{"sha256", metadata.SHA256},
						{"path", path},
					},
				}}
			}
		}
		return []txn.Op{op}, nil
	}
	txnRunner, session := s.txnRunnerWithSession()
	defer session.Close()
	err := txnRunner.Run(buildTxn)
	if err != nil {
		return errors.Annotate(err, "cannot store image metadata")
	}

	if oldPath != "" && oldPath != path {
		// Attempt to remove the old path. Failure is non-fatal.
		err := managedStorage.RemoveForEnvironment(s.envUUID, oldPath)
		if err != nil {
			logger.Errorf("failed to remove old image blob: %v", err)
		} else {
			logger.Debugf("removed old image blob")
		}
	}
	return nil
}

// DeleteImage is defined on the Storage interface.
func (s *imageStorage) DeleteImage(metadata *Metadata) (resultErr error) {
	path := imagePath(metadata.Kind, metadata.Series, metadata.Arch, metadata.SHA256)
	if err := s.getManagedStorage().RemoveForEnvironment(s.envUUID, path); err != nil {
		return errors.Annotate(err, "cannot remove image blob")
	}
	// Remove the metadata.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:      imagemetadataC,
			Id:     docId(metadata),
			Remove: true,
		}
		return []txn.Op{op}, nil
	}
	txnRunner, session := s.txnRunnerWithSession()
	defer session.Close()
	err := txnRunner.Run(buildTxn)
	// Metadata already removed, we don't care.
	if err == mgo.ErrNotFound {
		return nil
	}
	return errors.Annotate(err, "cannot remove image metadata")
}

// Image is defined on the Storage interface.
func (s *imageStorage) Image(kind, series, arch string) (*Metadata, io.ReadCloser, error) {
	metadataDoc, err := s.imageMetadataDoc(s.envUUID, kind, series, arch)
	if err != nil {
		return nil, nil, err
	}
	created, err := time.Parse(time.RFC3339, metadataDoc.Created)
	if err != nil {
		return nil, nil, err
	}
	image, err := s.imageBlob(s.getManagedStorage(), metadataDoc.Path)
	if err != nil {
		return nil, nil, err
	}
	metadata := &Metadata{
		EnvUUID: s.envUUID,
		Kind:    metadataDoc.Kind,
		Series:  metadataDoc.Series,
		Arch:    metadataDoc.Arch,
		Size:    metadataDoc.Size,
		SHA256:  metadataDoc.SHA256,
		Created: created,
	}
	return metadata, image, nil
}

type imageMetadataDoc struct {
	Id      string `bson:"_id"`
	Kind    string `bson:"kind"`
	Series  string `bson:"series"`
	Arch    string `bson:"arch"`
	Size    int64  `bson:"size"`
	SHA256  string `bson:"sha256"`
	Path    string `bson:"path"`
	Created string `bson:"created"`
}

func (s *imageStorage) imageMetadataDoc(envUUID, kind, series, arch string) (imageMetadataDoc, error) {
	var doc imageMetadataDoc
	id := fmt.Sprintf("%s-%s-%s-%s", envUUID, kind, series, arch)
	coll, closer := mongo.CollectionFromName(s.metadataCollection.Database, imagemetadataC)
	defer closer()
	err := coll.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return doc, errors.NotFoundf("%v image metadata", id)
	} else if err != nil {
		return doc, err
	}
	return doc, nil
}

func (s *imageStorage) imageBlob(managedStorage blobstore.ManagedStorage, path string) (io.ReadCloser, error) {
	r, _, err := managedStorage.GetForEnvironment(s.envUUID, path)
	return r, err
}

// imagePath returns the storage path for the specified image.
func imagePath(kind, series, arch, checksum string) string {
	return fmt.Sprintf("images/%s-%s-%s:%s", kind, series, arch, checksum)
}

// docId returns an id for the mongo image metadata document.
func docId(metadata *Metadata) string {
	return fmt.Sprintf("%s-%s-%s-%s", metadata.EnvUUID, metadata.Kind, metadata.Series, metadata.Arch)
}
