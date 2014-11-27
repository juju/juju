// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/storage"
)

// datastoreDoc represents storage associated with a unit.
type datastoreDoc struct {
	DocID   bson.ObjectId `bson:"_id"`
	Name    string        `bson:"name"`
	EnvUUID string        `bson:"env-uuid"`
	Unit    string        `bson:"unit"`

	Kind          int                  `bson:"kind"`
	Specification storageSpecification `bson:"spec"`
	Filesystem    *Filesystem          `bson:"filesystem,omitempty"`
}

type storageSpecification struct {
	Source                string                          `bson:"source"`
	Size                  uint64                          `bson:"size"`
	Options               string                          `bson:"options,omitempty"`
	ReadOnly              bool                            `bson:"readonly"`
	Persistent            bool                            `bson:"persistent"`
	FilesystemPreferences []datastoreFilesystemPreference `bson:"fsprefs,omitempty"`
}

type datastoreFilesystemPreference struct {
	Type         string   `bson:"type"`
	MountOptions []string `bson:"mountoptions,omitempty"`
	MkfsOptions  []string `bson:"mkfsoptions,omitempty"`
}

// Filesystem describes a datastore's filesystem.
type Filesystem struct {
	// Type is the filesystem type (ext4, xfs, etc.).
	Type string `bson:"type"`

	// MountOptions is any options to pass to the mount command.
	MountOptions []string `bson:"options,omitempty"`
}

func (st *State) Datastore(datastoreName string) (*storage.Datastore, error) {
	datastores, cleanup := st.getCollection(datastoresC)
	defer cleanup()

	var doc datastoreDoc
	err := datastores.FindId(st.docID(datastoreName)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("datastore %q", datastoreName)
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get datastore details")
	}
	result := toDatastores([]datastoreDoc{doc})[0]
	return &result, nil
}

// SetDatastoreFilesystem sets the datastore's filesystem.
func (st *State) SetDatastoreFilesystem(datastoreName string, fs Filesystem) error {
	ops := []txn.Op{{
		C:  datastoresC,
		Id: st.docID(datastoreName),
		// doc exists but filesystem is unset.
		// TODO(axw) ensure datastore is alive.
		Assert: bson.D{{"filesystem", nil}},
	}}
	err := st.runTransaction(ops)
	if err != nil {
		if err == txn.ErrAborted {
			datastore, err := st.Datastore(datastoreName)
			if err != nil {
				return err
			}
			if datastore.Filesystem != nil {
				return errors.Errorf("filesystem already set for datastore %q", datastoreName)
			}
		}
		return errors.Annotatef(err, "cannot set filesystem for datastore %q", datastoreName)
	}
	return nil
}

func fromDatastores(datastores []storage.Datastore) []datastoreDoc {
	docs := make([]datastoreDoc, len(datastores))
	for i, d := range datastores {
		docs[i] = datastoreDoc{
			Name: d.Name,
			Kind: int(d.Kind),
			// TODO the rest
		}
	}
	return docs
}

func toDatastores(docs []datastoreDoc) []storage.Datastore {
	datastores := make([]storage.Datastore, len(docs))
	for i, doc := range docs {
		datastores[i] = storage.Datastore{
			Name: doc.Name,
			Kind: storage.DatastoreKind(doc.Kind),
			// TODO the rest
		}
	}
	return datastores
}
