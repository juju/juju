// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package binarystorage

import (
	"fmt"
	"io"

	"github.com/juju/blobstore/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/internal/mongo"
)

var logger = loggo.GetLogger("juju.state.binarystorage")

type binaryStorage struct {
	modelUUID          string
	managedStorage     blobstore.ManagedStorage
	metadataCollection mongo.Collection
	txnRunner          jujutxn.Runner
}

var _ Storage = (*binaryStorage)(nil)

// New constructs a new Storage that stores binary files in the provided
// ManagedStorage, and metadata in the provided collection using the provided
// transaction runner.
func New(
	modelUUID string,
	managedStorage blobstore.ManagedStorage,
	metadataCollection mongo.Collection,
	runner jujutxn.Runner,
) Storage {
	return &binaryStorage{
		modelUUID:          modelUUID,
		managedStorage:     managedStorage,
		metadataCollection: metadataCollection,
		txnRunner:          runner,
	}
}

// Add implements Storage.Add.
func (s *binaryStorage) Add(r io.Reader, metadata Metadata) (resultErr error) {
	// Add the binary file to storage.
	path := fmt.Sprintf("tools/%s-%s", metadata.Version, metadata.SHA256)
	if err := s.managedStorage.PutForBucket(s.modelUUID, path, r, metadata.Size); err != nil {
		return errors.Annotate(err, "cannot store binary file")
	}
	defer func() {
		if resultErr == nil {
			return
		}
		err := s.managedStorage.RemoveForBucket(s.modelUUID, path)
		if err != nil {
			logger.Errorf("failed to remove binary blob: %v", err)
		}
	}()

	newDoc := metadataDoc{
		Id:      metadata.Version,
		Version: metadata.Version,
		Size:    metadata.Size,
		SHA256:  metadata.SHA256,
		Path:    path,
	}

	// Add or replace metadata. If replacing, record the existing path so we
	// can remove it later.
	var oldPath string
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:  s.metadataCollection.Name(),
			Id: newDoc.Id,
		}

		// On the first attempt we assume we're adding new binary files.
		// Subsequent attempts to add files will fetch the existing
		// doc, record the old path, and attempt to update the
		// size, path and hash fields.
		if attempt == 0 {
			op.Assert = txn.DocMissing
			op.Insert = &newDoc
		} else {
			oldDoc, err := s.findMetadata(metadata.Version)
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
	err := s.txnRunner.Run(buildTxn)
	if err != nil {
		return errors.Annotate(err, "cannot store binary metadata")
	}

	if oldPath != "" && oldPath != path {
		// Attempt to remove the old path. Failure is non-fatal.
		err := s.managedStorage.RemoveForBucket(s.modelUUID, oldPath)
		if err != nil {
			logger.Errorf("failed to remove old binary blob: %v", err)
		} else {
			logger.Debugf("removed old binary blob")
		}
	}
	return nil
}

func (s *binaryStorage) Open(version string) (Metadata, io.ReadCloser, error) {
	metadataDoc, err := s.findMetadata(version)
	if err != nil {
		return Metadata{}, nil, err
	}
	r, _, err := s.managedStorage.GetForBucket(s.modelUUID, metadataDoc.Path)
	if err != nil {
		return Metadata{}, nil, err
	}
	metadata := Metadata{
		Version: metadataDoc.Version,
		Size:    metadataDoc.Size,
		SHA256:  metadataDoc.SHA256,
	}
	return metadata, r, nil
}

func (s *binaryStorage) Metadata(version string) (Metadata, error) {
	metadataDoc, err := s.findMetadata(version)
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{
		Version: metadataDoc.Version,
		Size:    metadataDoc.Size,
		SHA256:  metadataDoc.SHA256,
	}, nil
}

func (s *binaryStorage) AllMetadata() ([]Metadata, error) {
	var docs []metadataDoc
	if err := s.metadataCollection.Find(nil).All(&docs); err != nil {
		return nil, err
	}
	list := make([]Metadata, len(docs))
	for i, doc := range docs {
		list[i] = Metadata{
			Version: doc.Version,
			Size:    doc.Size,
			SHA256:  doc.SHA256,
		}
	}
	return list, nil
}

type metadataDoc struct {
	Id      string `bson:"_id"`
	Version string `bson:"version"`
	Size    int64  `bson:"size"`
	SHA256  string `bson:"sha256,omitempty"`
	Path    string `bson:"path"`
}

func (s *binaryStorage) findMetadata(version string) (metadataDoc, error) {
	var doc metadataDoc
	err := s.metadataCollection.FindId(version).One(&doc)
	if err == mgo.ErrNotFound {
		return doc, errors.NotFoundf("%v binary metadata", version)
	}
	return doc, err
}
