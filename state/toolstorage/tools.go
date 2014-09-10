// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolstorage

import (
	"fmt"
	"io"

	"github.com/juju/blobstore"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/version"
)

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

func (s *toolsStorage) AddTools(r io.Reader, metadata Metadata) error {
	// Add the tools tarball to storage.
	path := toolsPath(metadata.Version, metadata.SHA256)
	if err := s.managedStorage.PutForEnvironment(s.envUUID, path, r, metadata.Size); err != nil {
		return errors.Annotate(err, "cannot store tools tarball")
	}

	// Add or replace metadata.
	doc := toolsMetadataDoc{
		Id:      metadata.Version.String(),
		Version: metadata.Version,
		Size:    metadata.Size,
		SHA256:  metadata.SHA256,
		Path:    path,
	}
	ops := []txn.Op{{
		C:      s.metadataCollection.Name,
		Id:     doc.Id,
		Insert: &doc,
	}, {
		C:  s.metadataCollection.Name,
		Id: doc.Id,
		Update: bson.D{{
			"$set", bson.D{
				{"size", metadata.Size},
				{"sha256", metadata.SHA256},
				{"path", path},
			},
		}},
	}}
	// TODO(axw) if replacing existing metadata, remove the blob if
	// there are no other metadata (e.g. aliases) still referencing it.
	err := s.txnRunner.RunTransaction(ops)
	if err != nil {
		return errors.Annotate(err, "cannot store tools metadata")
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
