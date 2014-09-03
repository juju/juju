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

// Metadata describes a Juju tools tarball.
type Metadata struct {
	Version version.Binary
	Size    int64
	SHA256  string
}

// Storage provides methods for storing and retrieving tools by version.
type Storage interface {
	// AddTools adds the tools tarball and metadata into state,
	// failing if there already exist tools with the specified
	// version, replacing existing metadata if any exist with
	// the specified version.
	AddTools(io.Reader, Metadata) error

	// AddToolsAlias adds an alias for the tools with the specified version,
	// failing if metadata already exists for the alias version.
	AddToolsAlias(alias, version version.Binary) error

	// AllMetadata returns metadata for the full list of tools in
	// the catalogue.
	AllMetadata() ([]Metadata, error)

	// Tools returns the Metadata and tools tarball contents
	// for the specified version if it exists, else an error
	// satisfying errors.IsNotFound.
	Tools(version.Binary) (Metadata, io.ReadCloser, error)

	// Metadata returns the Metadata for the specified version
	// if it exists, else an error satisfying errors.IsNotFound.
	Metadata(v version.Binary) (Metadata, error)
}

// StorageCloser extends the Storage interface with a Close method.
type StorageCloser interface {
	Storage
	Close() error
}

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
		return err
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
	return s.txnRunner.RunTransaction(ops)
}

// AddToolsAlias adds an alias for the tools with the specified version,
// failing if metadata already exists for the alias version.
func (s *toolsStorage) AddToolsAlias(alias, version version.Binary) error {
	existingDoc, err := s.toolsMetadata(version)
	if err != nil {
		return err
	}
	newDoc := toolsMetadataDoc{
		Id:      alias.String(),
		Version: alias,
		Size:    existingDoc.Size,
		SHA256:  existingDoc.SHA256,
		Path:    existingDoc.Path,
	}
	ops := []txn.Op{{
		C:      s.metadataCollection.Name,
		Id:     newDoc.Id,
		Assert: txn.DocMissing,
		Insert: &newDoc,
	}}
	err = s.txnRunner.RunTransaction(ops)
	if err == txn.ErrAborted {
		return errors.AlreadyExistsf("%v tools metadata", alias)
	}
	return err
}

// Tools returns the Metadata and tools tarball contents
// for the specified version if it exists, else an error
// satisfying errors.IsNotFound.
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

// Metadata returns the Metadata for the specified version
// if it exists, else an error satisfying errors.IsNotFound.
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

// AllMetadata returns metadata for the full list of tools in
// the catalogue.
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
