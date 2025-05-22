// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package binarystorage

import (
	"context"
	"fmt"
	"io"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/mongo"
)

var logger = internallogger.GetLogger("juju.state.binarystorage")

// ManagedStorage instances persist data for a bucket, for a user, or globally.
// (Only bucket storage is currently implemented).
type ManagedStorage interface {
	// Get returns a reader for data at path, namespaced to the bucket.
	// If the data is still being uploaded and is not fully written yet,
	// an ErrUploadPending error is returned. This means the path is valid but
	// the caller should try again to retrieve the data.
	Get(ctx context.Context, path string) (r io.ReadCloser, length int64, err error)

	// Put stores data from reader at path, namespaced to the bucket.
	Put(ctx context.Context, path string, r io.Reader, length int64) (objectstore.UUID, error)

	// Remove deletes data at path, namespaced to the bucket.
	Remove(ctx context.Context, path string) error
}

type binaryStorage struct {
	managedStorage     ManagedStorage
	metadataCollection mongo.Collection
	txnRunner          jujutxn.Runner
}

var _ Storage = (*binaryStorage)(nil)

// New constructs a new Storage that stores binary files in the provided
// ManagedStorage, and metadata in the provided collection using the provided
// transaction runner.
func New(
	managedStorage ManagedStorage,
	metadataCollection mongo.Collection,
	runner jujutxn.Runner,
) Storage {
	return &binaryStorage{
		managedStorage:     managedStorage,
		metadataCollection: metadataCollection,
		txnRunner:          runner,
	}
}

// Add implements Storage.Add.
func (s *binaryStorage) Add(ctx context.Context, r io.Reader, metadata Metadata) (resultErr error) {
	// Add the binary file to storage.
	path := fmt.Sprintf("tools/%s-%s", metadata.Version, metadata.SHA256)
	if _, err := s.managedStorage.Put(ctx, path, r, metadata.Size); err != nil &&
		!errors.Is(err, objectstoreerrors.ErrPathAlreadyExistsDifferentHash) &&
		!errors.Is(err, objectstoreerrors.ErrHashAndSizeAlreadyExists) {
		return errors.Annotate(err, "cannot store binary file")
	}
	defer func() {
		if resultErr == nil {
			return
		}
		err := s.managedStorage.Remove(ctx, path)
		if err != nil && !errors.Is(err, objectstoreerrors.ErrNotFound) {
			logger.Errorf(context.TODO(), "failed to remove binary blob: %v", err)
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
			op.Assert = bson.D{{Name: "path", Value: oldPath}}
			if oldPath != path {
				op.Update = bson.D{{
					Name: "$set", Value: bson.D{
						{Name: "size", Value: metadata.Size},
						{Name: "sha256", Value: metadata.SHA256},
						{Name: "path", Value: path},
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
		err := s.managedStorage.Remove(ctx, oldPath)
		if err != nil {
			logger.Errorf(context.TODO(), "failed to remove old binary blob: %v", err)
		} else {
			logger.Debugf(context.TODO(), "removed old binary blob")
		}
	}
	return nil
}

func (s *binaryStorage) Open(ctx context.Context, version string) (Metadata, io.ReadCloser, error) {
	metadataDoc, err := s.findMetadata(version)
	if err != nil {
		return Metadata{}, nil, errors.Trace(err)
	}
	r, _, err := s.managedStorage.Get(ctx, metadataDoc.Path)
	if err != nil {
		return Metadata{}, nil, errors.Annotatef(err, "resource at %q", metadataDoc.Path)
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
