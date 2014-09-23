// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage

import (
	"fmt"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/filestorage"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/metadata"
)

// NewID returns a new ID for a state backup.  The format is the
// UTC timestamp from the metadata followed by the environment ID:
// "YYYYMMDD-hhmmss.<env ID>".  This makes the ID a little more human-
// consumable (in contrast to a plain UUID string).  Ideally we would
// use some form of environment name rather than the UUID, but for now
// the raw env ID is sufficient.
func NewID(metadata *metadata.Metadata) string {
	rawts := metadata.Started()
	Y, M, D := rawts.Date()
	h, m, s := rawts.Clock()
	timestamp := fmt.Sprintf("%04d%02d%02d-%02d%02d%02d", Y, M, D, h, m, s)
	origin := metadata.Origin()
	env := origin.Environment()
	return timestamp + "." + env
}

// NewStorage returns a new FileStorage to use for storing backup
// archives (and metadata).
func NewStorage(st *state.State, coll *mgo.Collection, txnRunner jujutxn.Runner) (filestorage.FileStorage, error) {
	envStor, err := environs.GetStorage(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	files := newEnvFileStorage(envStor, envStorageRoot)
	docs := NewMetadataStorage(coll, txnRunner)
	return filestorage.NewFileStorage(docs, files), nil
}
