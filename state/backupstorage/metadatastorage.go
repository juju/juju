// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/filestorage"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// Ensure we satisfy the interface.
var _ filestorage.MetadataStorage = (*metadataStorage)(nil)

type metadataStorage struct {
	filestorage.MetadataDocStorage
	coll      *mgo.Collection
	txnRunner jujutxn.Runner
}

// NewMetadataStorage returns a new metadata storage.
func NewMetadataStorage(coll *mgo.Collection, txnRunner jujutxn.Runner) filestorage.MetadataStorage {
	docStor := NewDocStorage(coll, txnRunner)
	stor := metadataStorage{
		MetadataDocStorage: filestorage.MetadataDocStorage{docStor},
		coll:               coll,
		txnRunner:          txnRunner,
	}
	return &stor
}

// SetStored records in the metadata the fact that the file was stored.
func (s *metadataStorage) SetStored(id string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		ops := []txn.Op{{
			C:      s.coll.Name,
			Id:     id,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"stored", true},
			}}},
		}}
		return ops, nil
	}
	if err := s.txnRunner.Run(buildTxn); err != nil {
		if err == txn.ErrAborted {
			return errors.NotFoundf(id)
		}
		return errors.Annotate(err, "error running transaction")
	}
	return nil
}
