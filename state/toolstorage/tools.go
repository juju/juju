// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolstorage

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

var logger = loggo.GetLogger("juju.state.toolstorage")

type toolsStorage struct {
	envUUID            string
	managedStorage     blobstore.ManagedStorage
	metadataCollection *mgo.Collection
	txnRunner          jujutxn.Runner
}

var _ Storage = (*toolsStorage)(nil)

// NewStorage constructs a new Storage that stores tools tarballs
// in the provided ManagedStorage, and tools metadata in the provided
// collection using the provided transaction runner.
func NewStorage(
	envUUID string,
	managedStorage blobstore.ManagedStorage,
	metadataCollection *mgo.Collection,
	runner jujutxn.Runner,
) Storage {
	return &toolsStorage{
		envUUID:            envUUID,
		managedStorage:     managedStorage,
		metadataCollection: metadataCollection,
		txnRunner:          runner,
	}
}

func (s *toolsStorage) AddTools(r io.Reader, metadata Metadata) (resultErr error) {
	// Add the tools tarball to storage.
	path := toolsPath(metadata.Version, metadata.SHA256)
	if err := s.managedStorage.PutForEnvironment(s.envUUID, path, r, metadata.Size); err != nil {
		return errors.Annotate(err, "cannot store tools tarball")
	}
	defer func() {
		if resultErr == nil {
			return
		}
		err := s.managedStorage.RemoveForEnvironment(s.envUUID, path)
		if err != nil {
			logger.Errorf("failed to remove tools blob: %v", err)
		}
	}()

	newDoc := toolsMetadataDoc{
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

		// On the first attempt we assume we're adding new tools.
		// Subsequent attempts to add tools will fetch the existing
		// doc, record the old path, and attempt to update the
		// size, path and hash fields.
		if attempt == 0 {
			op.Assert = txn.DocMissing
			op.Insert = &newDoc
		} else {
			oldDoc, err := s.toolsMetadata(metadata.Version)
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
		return errors.Annotate(err, "cannot store tools metadata")
	}

	if oldPath != "" && oldPath != path {
		// Attempt to remove the old path. Failure is non-fatal.
		err := s.managedStorage.RemoveForEnvironment(s.envUUID, oldPath)
		if err != nil {
			logger.Errorf("failed to remove old tools blob: %v", err)
		} else {
			logger.Debugf("removed old tools blob")
		}
	}
	return nil
}

func (s *toolsStorage) Tools(v version.Binary) (Metadata, io.ReadCloser, error) {
	metadataDoc, err := s.toolsMetadata(v)
	if err != nil {
		return Metadata{}, nil, err
	}
	tools, err := s.toolsTarball(metadataDoc.Path)
	if err != nil {
		return Metadata{}, nil, err
	}
	metadata := Metadata{
		Version: metadataDoc.Version,
		Size:    metadataDoc.Size,
		SHA256:  metadataDoc.SHA256,
	}
	return metadata, tools, nil
}

func (s *toolsStorage) Metadata(v version.Binary) (Metadata, error) {
	metadataDoc, err := s.toolsMetadata(v)
	if err != nil {
		return Metadata{}, err
	}
	metadata := Metadata{
		Version: metadataDoc.Version,
		Size:    metadataDoc.Size,
		SHA256:  metadataDoc.SHA256,
	}
	return metadata, nil
}

func (s *toolsStorage) AllMetadata() ([]Metadata, error) {
	var docs []toolsMetadataDoc
	if err := s.metadataCollection.Find(nil).All(&docs); err != nil {
		return nil, err
	}
	list := make([]Metadata, len(docs))
	for i, doc := range docs {
		metadata := Metadata{
			Version: doc.Version,
			Size:    doc.Size,
			SHA256:  doc.SHA256,
		}
		list[i] = metadata
	}
	return list, nil
}

type toolsMetadataDoc struct {
	Id      string         `bson:"_id"`
	Version version.Binary `bson:"version"`
	Size    int64          `bson:"size"`
	SHA256  string         `bson:"sha256,omitempty"`
	Path    string         `bson:"path"`
}

func (s *toolsStorage) toolsMetadata(v version.Binary) (toolsMetadataDoc, error) {
	var doc toolsMetadataDoc
	err := s.metadataCollection.Find(bson.D{{"_id", v.String()}}).One(&doc)
	if err == mgo.ErrNotFound {
		return doc, errors.NotFoundf("%v tools metadata", v)
	} else if err != nil {
		return doc, err
	}
	return doc, nil
}

func (s *toolsStorage) toolsTarball(path string) (io.ReadCloser, error) {
	r, _, err := s.managedStorage.GetForEnvironment(s.envUUID, path)
	return r, err
}

// toolsPath returns the storage path for the specified tools.
func toolsPath(v version.Binary, hash string) string {
	return fmt.Sprintf("tools/%s-%s", v, hash)
}
