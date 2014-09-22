// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage

import (
	"github.com/juju/errors"
	"github.com/juju/utils/filestorage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/metadata"
)

// Ensure we satisfy the interface.
var _ filestorage.DocStorage = (*docStorage)(nil)

type docStorage struct {
	state *state.State
}

// NewDocStorage returns a new doc storage.
func NewDocStorage(st *state.State) filestorage.DocStorage {
	return &docStorage{state: st}
}

func (s *docStorage) AddDoc(doc filestorage.Doc) (string, error) {
	meta, ok := doc.(*metadata.Metadata)
	if !ok {
		return "", errors.Errorf("doc must be of type state.backups.metadata.Metadata")
	}
	return s.addMetadata(meta)
}

// addBackupMetadata stores metadata for a backup where it can be
// accessed later.  It returns a new ID that is associated with the
// backup.  If the provided metadata already has an ID set, it is
// ignored.
func (s *docStorage) addMetadata(meta *metadata.Metadata) (string, error) {
	// We use our own mongo _id value since the auto-generated one from
	// mongo may contain sensitive data (see bson.ObjectID).
	id := NewID(meta)
	return id, s.addMetadataID(meta, id)
}

func (s *docStorage) addMetadataID(meta *metadata.Metadata, id string) error {
	var doc metadataDoc
	doc.updateFromMetadata(meta)
	doc.ID = id
	if err := doc.validate(); err != nil {
		return errors.Trace(err)
	}

	ops := []txn.Op{{
		C:      state.BackupsMetaC,
		Id:     doc.ID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
	if err := state.RunTransaction(s.state, ops); err != nil {
		if err == txn.ErrAborted {
			return errors.AlreadyExistsf("backup metadata %q", doc.ID)
		}
		return errors.Annotate(err, "error running transaction")
	}

	return nil
}

func (s *docStorage) Doc(id string) (filestorage.Doc, error) {
	collection, closer := state.GetCollection(s.state, state.BackupsMetaC)
	defer closer()

	var doc metadataDoc
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

func (s *docStorage) ListDocs() ([]filestorage.Doc, error) {
	// This will be implemented when backups needs this functionality.
	// For now the method is stubbed out for the same of the
	// MetadataStorage interface.
	return nil, errors.NotImplementedf("ListDocs")
}

func (s *docStorage) RemoveDoc(id string) error {
	// This will be implemented when backups needs this functionality.
	// For now the method is stubbed out for the same of the
	// MetadataStorage interface.
	return errors.NotImplementedf("RemoveDoc")
}

// Close implements io.Closer.Close.
func (s *docStorage) Close() error {
	return nil
}
