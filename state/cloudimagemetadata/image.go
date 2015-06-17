// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	"fmt"
	"io"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.state.cloudimagemetadata")

type cloudImageStorage struct {
	envUUID            string
	managedStorage     blobstore.ManagedStorage
	metadataCollection *mgo.Collection
	txnRunner          jujutxn.Runner
}

var _ Storage = (*cloudImageStorage)(nil)

// NewCloudImageStorage constructs a new Storage that stores images tarballs
// in the provided ManagedStorage, and image metadata in the provided
// collection using the provided transaction runner.
func NewCloudImageStorage(
	envUUID string,
	managedStorage blobstore.ManagedStorage,
	metadataCollection *mgo.Collection,
	runner jujutxn.Runner,
) Storage {
	return &cloudImageStorage{
		envUUID:            envUUID,
		managedStorage:     managedStorage,
		metadataCollection: metadataCollection,
		txnRunner:          runner,
	}
}

func (s *cloudImageStorage) AddCloudImages(r io.Reader, metadata Metadata) (resultErr error) {
	// Add the images tarball to storage.
	path := imagesPath(metadata.Version, metadata.SHA256)
	err := s.managedStorage.PutForEnvironment(s.envUUID, path, r, metadata.Size)
	if err != nil {
		return errors.Annotate(err, "cannot store cloud images tarball")
	}
	defer func() {
		if resultErr != nil {
			err := s.managedStorage.RemoveForEnvironment(s.envUUID, path)
			if err != nil {
				logger.Errorf("failed to remove cloud images blob: %v", err)
			}
		}
	}()

	newDoc := imagesMetadataDoc{
		Id:      metadata.Version.String(),
		Version: metadata.Version,
		Size:    metadata.Size,
		SHA256:  metadata.SHA256,
		Path:    path,
	}

	// Add or replace metadata. If replacing, record the
	// existing path so we can remove it later.
	var oldPath string
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:  s.metadataCollection.Name,
			Id: newDoc.Id,
		}

		// On the first attempt we assume we're adding new images.
		// Subsequent attempts to add images will fetch the existing
		// doc, record the old path, and attempt to update the
		// size, path and hash fields.
		if attempt == 0 {
			op.Assert = txn.DocMissing
			op.Insert = &newDoc
		} else {
			oldDoc, err := s.imagesMetadata(metadata.Version)
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
	err = s.txnRunner.Run(buildTxn)
	if err != nil {
		return errors.Annotate(err, "cannot store cloud images metadata")
	}

	if oldPath != "" && oldPath != path {
		// Attempt to remove the old path. Failure is non-fatal.
		err := s.managedStorage.RemoveForEnvironment(s.envUUID, oldPath)
		if err != nil {
			logger.Errorf("failed to remove old cloud images blob: %v", err)
		} else {
			logger.Debugf("removed old cloud images blob")
		}
	}
	return nil
}

func (s *cloudImageStorage) CloudImages(v version.Binary) (Metadata, io.ReadCloser, error) {
	metadataDoc, err := s.imagesMetadata(v)
	if err != nil {
		return Metadata{}, nil, err
	}
	images, err := s.imagesTarball(metadataDoc.Path)
	if err != nil {
		return Metadata{}, nil, err
	}
	return metadataDoc.public(), images, nil
}

func (s *cloudImageStorage) Metadata(v version.Binary) (Metadata, error) {
	metadataDoc, err := s.imagesMetadata(v)
	if err != nil {
		return Metadata{}, errors.Trace(err)
	}
	return metadataDoc.public(), nil
}

func (s *cloudImageStorage) AllMetadata() ([]Metadata, error) {
	var docs []imagesMetadataDoc
	if err := s.metadataCollection.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	metadata := make([]Metadata, len(docs))
	for i, doc := range docs {
		metadata[i] = doc.public()
	}
	return metadata, nil
}

func (s *cloudImageStorage) imagesMetadata(v version.Binary) (imagesMetadataDoc, error) {
	var doc imagesMetadataDoc
	err := s.metadataCollection.Find(bson.D{{"_id", v.String()}}).One(&doc)
	if err == mgo.ErrNotFound {
		return doc, errors.NotFoundf("%v cloud images metadata", v)
	}
	if err != nil {
		return doc, err
	}
	return doc, nil
}

func (s *cloudImageStorage) imagesTarball(path string) (io.ReadCloser, error) {
	r, _, err := s.managedStorage.GetForEnvironment(s.envUUID, path)
	return r, err
}

type imagesMetadataDoc struct {
	Id      string         `bson:"_id"`
	Version version.Binary `bson:"version"`
	Size    int64          `bson:"size"`
	SHA256  string         `bson:"sha256,omitempty"`
	Path    string         `bson:"path"`
}

func (m imagesMetadataDoc) public() Metadata {
	return Metadata{
		Version: m.Version,
		Size:    m.Size,
		SHA256:  m.SHA256,
	}
}

// imagesPath returns the storage path for the specified cloud images.
func imagesPath(v version.Binary, hash string) string {
	return fmt.Sprintf("cloudimages/%s-%s", v, hash)
}
