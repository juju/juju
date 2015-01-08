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

func (s *imageStorage) getManagedStorage(session *mgo.Session) blobstore.ManagedStorage {
	return getManagedStorage(session)
}

func (s *imageStorage) txnRunner(session *mgo.Session) jujutxn.Runner {
	db := s.metadataCollection.Database
	runnerDb := db.With(session)
	return txnRunner(runnerDb)
}

// Override for testing.
var txnRunner = func(db *mgo.Database) jujutxn.Runner {
	return jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
}

// AddImage is defined on the Storage interface.
func (s *imageStorage) AddImage(r io.Reader, metadata *Metadata) (resultErr error) {
	session := s.blobDb.Session.Copy()
	defer session.Close()
	managedStorage := s.getManagedStorage(session)
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
		Id:        docId(metadata),
		EnvUUID:   s.envUUID,
		Kind:      metadata.Kind,
		Series:    metadata.Series,
		Arch:      metadata.Arch,
		Size:      metadata.Size,
		SHA256:    metadata.SHA256,
		SourceURL: metadata.SourceURL,
		Path:      path,
		Created:   time.Now(),
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
	txnRunner := s.txnRunner(session)
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

// ListImages is defined on the Storage interface.
func (s *imageStorage) ListImages(filter ImageFilter) ([]*Metadata, error) {
	metadataDocs, err := s.listImageMetadataDocs(s.envUUID, filter.Kind, filter.Series, filter.Arch)
	if err != nil {
		return nil, errors.Annotate(err, "cannot list image metadata")
	}
	result := make([]*Metadata, len(metadataDocs))
	for i, metadataDoc := range metadataDocs {
		result[i] = &Metadata{
			EnvUUID:   s.envUUID,
			Kind:      metadataDoc.Kind,
			Series:    metadataDoc.Series,
			Arch:      metadataDoc.Arch,
			Size:      metadataDoc.Size,
			SHA256:    metadataDoc.SHA256,
			Created:   metadataDoc.Created,
			SourceURL: metadataDoc.SourceURL,
		}
	}
	return result, nil
}

// DeleteImage is defined on the Storage interface.
func (s *imageStorage) DeleteImage(metadata *Metadata) (resultErr error) {
	session := s.blobDb.Session.Copy()
	defer session.Close()
	managedStorage := s.getManagedStorage(session)
	path := imagePath(metadata.Kind, metadata.Series, metadata.Arch, metadata.SHA256)
	if err := managedStorage.RemoveForEnvironment(s.envUUID, path); err != nil {
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
	txnRunner := s.txnRunner(session)
	err := txnRunner.Run(buildTxn)
	// Metadata already removed, we don't care.
	if err == mgo.ErrNotFound {
		return nil
	}
	return errors.Annotate(err, "cannot remove image metadata")
}

// imageCloser encapsulates an image reader and session
// so that both are closed together.
type imageCloser struct {
	io.ReadCloser
	session *mgo.Session
}

func (c *imageCloser) Close() error {
	c.session.Close()
	return c.ReadCloser.Close()
}

// Image is defined on the Storage interface.
func (s *imageStorage) Image(kind, series, arch string) (*Metadata, io.ReadCloser, error) {
	metadataDoc, err := s.imageMetadataDoc(s.envUUID, kind, series, arch)
	if err != nil {
		return nil, nil, err
	}
	session := s.blobDb.Session.Copy()
	managedStorage := s.getManagedStorage(session)
	image, err := s.imageBlob(managedStorage, metadataDoc.Path)
	if err != nil {
		return nil, nil, err
	}
	metadata := &Metadata{
		EnvUUID:   s.envUUID,
		Kind:      metadataDoc.Kind,
		Series:    metadataDoc.Series,
		Arch:      metadataDoc.Arch,
		Size:      metadataDoc.Size,
		SHA256:    metadataDoc.SHA256,
		SourceURL: metadataDoc.SourceURL,
		Created:   metadataDoc.Created,
	}
	imageResult := &imageCloser{
		image,
		session,
	}
	return metadata, imageResult, nil
}

type imageMetadataDoc struct {
	Id        string    `bson:"_id"`
	EnvUUID   string    `bson:"envuuid"`
	Kind      string    `bson:"kind"`
	Series    string    `bson:"series"`
	Arch      string    `bson:"arch"`
	Size      int64     `bson:"size"`
	SHA256    string    `bson:"sha256"`
	Path      string    `bson:"path"`
	Created   time.Time `bson:"created"`
	SourceURL string    `bson:"sourceurl"`
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

func (s *imageStorage) listImageMetadataDocs(envUUID, kind, series, arch string) ([]imageMetadataDoc, error) {
	coll, closer := mongo.CollectionFromName(s.metadataCollection.Database, imagemetadataC)
	defer closer()
	imageDocs := []imageMetadataDoc{}
	filter := bson.D{{"envuuid", envUUID}}
	if kind != "" {
		filter = append(filter, bson.DocElem{"kind", kind})
	}
	if series != "" {
		filter = append(filter, bson.DocElem{"series", series})
	}
	if arch != "" {
		filter = append(filter, bson.DocElem{"arch", arch})
	}
	err := coll.Find(filter).All(&imageDocs)
	return imageDocs, err
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
