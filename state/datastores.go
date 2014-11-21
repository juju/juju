// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/storage"
)

// datastoreDoc represents storage associated with a unit.
type datastoreDoc struct {
	DocId         bson.ObjectId         `bson:"_id"`
	Id            string                `bson:"datastoreid"`
	EnvUUID       string                `bson:"env-uuid"`
	Unit          string                `bson:"unit"`
	Name          string                `bson:"name"`
	Kind          storage.DatastoreKind `bson:"kind"`
	Location      string                `bson:"location,omitempty"`
	Specification storageSpecification  `bson:"spec,omitempty"`
	Filesystem    *datastoreFilesystem  `bson:"filesystem,omitempty"`
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

type datastoreFilesystem struct {
	Type         string   `bson:"type"`
	MountOptions []string `bson:"options,omitempty"`
}
