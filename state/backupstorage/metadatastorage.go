// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage

import (
	"github.com/juju/errors"
	"github.com/juju/utils/filestorage"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state"
)

// Ensure we satisfy the interface.
var _ filestorage.MetadataStorage = (*metadataStorage)(nil)

type metadataStorage struct {
	filestorage.MetadataDocStorage
	state *state.State
}

// NewMetadataStorage returns a new metadata storage.
func NewMetadataStorage(st *state.State) filestorage.MetadataStorage {
	docStor := NewDocStorage(st)
	stor := metadataStorage{
		MetadataDocStorage: filestorage.MetadataDocStorage{docStor},
		state:              st,
	}
	return &stor
}

// SetStored records in the metadata the fact that the file was stored.
func (s *metadataStorage) SetStored(id string) error {
	ops := []txn.Op{{
		C:      state.BackupsMetaC,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"stored", true},
		}}},
	}}
	if err := state.RunTransaction(s.state, ops); err != nil {
		if err == txn.ErrAborted {
			return errors.NotFoundf(id)
		}
		return errors.Annotate(err, "error running transaction")
	}
	return nil
}
