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
)

var logger = loggo.GetLogger("juju.state.imagestorage")

type imageStorage struct {
	envUUID            string
	managedStorage     blobstore.ManagedStorage
	metadataCollection *mgo.Collection
	txnRunner          jujutxn.Runner
}

var _ Storage = (*imageStorage)(nil)

// NewStorage constructs a new Storage that stores image blobs
// in the provided ManagedStorage, and image metadata in the provided
// collection using the provided transaction runner.
func NewStorage(
	envUUID string,
	managedStorage blobstore.ManagedStorage,
	metadataCollection *mgo.Collection,
	runner jujutxn.Runner,
) Storage {
	return &imageStorage{
		envUUID:            envUUID,
		managedStorage:     managedStorage,
		metadataCollection: metadataCollection,
		txnRunner:          runner,
	}
}

// AddImage is defined on the Storage interface.
func (s *imageStorage) AddImage(r io.Reader, metadata *Metadata) (resultErr error) {
	path := imagePath(metadata.Kind, metadata.Series, metadata.Arch, metadata.Checksum)
	if err := s.managedStorage.PutForEnvironment(s.envUUID, path, r, metadata.Size); err != nil {
		return errors.Annotate(err, "cannot store image")
	}
	defer func() {
		if resultErr == nil {
			return
		}
		err := s.managedStorage.RemoveForEnvironment(s.envUUID, path)
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
		SHA256:  metadata.Checksum,
		Path:    path,
		Created: time.Now().Format(time.RFC3339),
	}

	// Add or replace metadata. If replacing, record the
	// existing path so we can remove it later.
	var oldPath string
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:  s.metadataCollection.Name,
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
			oldDoc, err := s.imageMetadataDoc(metadata.Kind, metadata.Series, metadata.Arch)
			if err != nil {
				return nil, err
			}
			oldPath = oldDoc.Path
			op.Assert = bson.D{{"path", oldPath}}
			if oldPath != path {
				op.Update = bson.D{{
					"$set", bson.D{
						{"size", metadata.Size},
						{"sha256", metadata.Checksum},
						{"path", path},
					},
				}}
			}
		}
		return []txn.Op{op}, nil
	}
	err := s.txnRunner.Run(buildTxn)
	if err != nil {
		return errors.Annotate(err, "cannot store image metadata")
	}

	if oldPath != "" && oldPath != path {
		// Attempt to remove the old path. Failure is non-fatal.
		err := s.managedStorage.RemoveForEnvironment(s.envUUID, oldPath)
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
	path := imagePath(metadata.Kind, metadata.Series, metadata.Arch, metadata.Checksum)
	if err := s.managedStorage.RemoveForEnvironment(s.envUUID, path); err != nil {
		return errors.Annotate(err, "cannot remove image blob")
	}
	// Remove the metadata.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:      s.metadataCollection.Name,
			Id:     docId(metadata),
			Remove: true,
		}
		return []txn.Op{op}, nil
	}
	err := s.txnRunner.Run(buildTxn)
	return errors.Annotate(err, "cannot remove image metadata")
}

// Image is defined on the Storage interface.
func (s *imageStorage) Image(kind, series, arch string) (*Metadata, io.ReadCloser, error) {
	metadataDoc, err := s.imageMetadataDoc(kind, series, arch)
	if err != nil {
		return nil, nil, err
	}
	created, err := time.Parse(time.RFC3339, metadataDoc.Created)
	if err != nil {
		return nil, nil, err
	}
	image, err := s.imageBlob(metadataDoc.Path)
	if err != nil {
		return nil, nil, err
	}
	metadata := &Metadata{
		Kind:     metadataDoc.Kind,
		Series:   metadataDoc.Series,
		Arch:     metadataDoc.Arch,
		Size:     metadataDoc.Size,
		Checksum: metadataDoc.SHA256,
		Created:  created,
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

func (s *imageStorage) imageMetadataDoc(kind, series, arch string) (imageMetadataDoc, error) {
	var doc imageMetadataDoc
	id := fmt.Sprintf("%s-%s-%s", kind, series, arch)
	err := s.metadataCollection.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return doc, errors.NotFoundf("%v image metadata", id)
	} else if err != nil {
		return doc, err
	}
	return doc, nil
}

func (s *imageStorage) imageBlob(path string) (io.ReadCloser, error) {
	r, _, err := s.managedStorage.GetForEnvironment(s.envUUID, path)
	return r, err
}

// imagePath returns the storage path for the specified image.
func imagePath(kind, series, arch, checksum string) string {
	return fmt.Sprintf("images/%s-%s-%s:%s", kind, series, arch, checksum)
}

// docId returns an id for the mongo image metadata document.
func docId(metadata *Metadata) string {
	return fmt.Sprintf("%s-%s-%s", metadata.Kind, metadata.Series, metadata.Arch)
}
