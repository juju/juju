// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
    "fmt"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/version"
)

/*
Backups are not a part of juju state nor of normal state operations.
However, they certainly are tightly coupled with state (the very
subject of backups).  This puts backups in an odd position,
particularly with regard to the storage of backup metadata and
archives.  As a result, here are a couple concerns worth mentioning.

First, as noted above backup is about state but not a part of state.
So exposing backup-related methods on State would imply the wrong
thing.  Thus the backup functionality here in the state package (not
state/backup) is exposed as functions to which you pass a state
object.

Second, backup creates an archive file containing a dump of state's
mongo DB.  Storing backup metadata/archives in mongo is thus a
somewhat circular proposition that has the potential to cause
problems.  That may need further attention.

Note that state (and juju as a whole) currently does not have a
persistence layer abstraction to facilitate separating different
persistence needs and implementations.  As a consequence, state's
data, whether about how an environment should look or about existing
resources within an environment, is dumped essentially straight into
State's mongo connection.  The code in the state package does not
make any distinction between the two (nor does the package clearly
distinguish between state-related abstractions and state-related
data).

Backup adds yet another category, merely taking advantage of
State's DB.  In the interest of making the distinction clear, the
code that directly interacts with State (and its DB) lives in this
file.  As mentioned previously, the functionality here is exposed
through functions that take State, rather than as methods on State.
Furthermore, the bulk of the backup code, which does not need direct
interaction with State, lives in the state/backup package.
*/

// NewBackupOrigin returns a snapshot of where backup was run.
func NewBackupOrigin(st *State, machine string) *backups.Origin {
	// hostname could be derived from the environment...
	hostname, err := os.Hostname()
	if err != nil {
		panic(fmt.Sprintf("could not get hostname: %v", err))
	}
	origin := backups.Origin{
		Environment: st.EnvironTag().Id(),
		Machine:     machine,
		Hostname:    hostname,
		Version:     version.Current.Number,
	}
	return &origin
}

// backupMetadataDoc is a mirror of backups.Metadata, used just for DB storage.
type backupMetadataDoc struct {
	ID             string `bson:"_id"`
	Started        int64  `bson:"started,minsize"`
	Finished       int64  `bson:"finished,minsize"`
	CheckSum       string `bson:"checksum"`
	CheckSumFormat string `bson:"checksumFormat"`
	Size           int64  `bson:"size,minsize"`
	Stored         bool   `bson:"stored"`
	Notes          string `bson:"notes,omitempty"`

	// origin
	Environment string         `bson:"environment"`
	Machine     string         `bson:"machine"`
	Hostname    string         `bson:"hostname"`
	Version     version.Number `bson:"version"`
}

// asMetadata returns a new backups.Metadata based on the backupMetadataDoc.
func (doc *backupMetadataDoc) asMetadata() *backups.Metadata {
	origin := backups.Origin{
		Environment: doc.Environment,
		Machine:     doc.Machine,
		Hostname:    doc.Hostname,
		Version:     doc.Version,
	}
	metadata := backups.Metadata{
		ID:             doc.ID,
		Timestamp:      time.Unix(doc.Started, 0).UTC(),
		Finished:       time.Unix(doc.Finished, 0).UTC(),
		CheckSum:       doc.CheckSum,
		CheckSumFormat: doc.CheckSumFormat,
		Size:           doc.Size,
		Origin:         origin,
		Stored:         doc.Stored,
		Notes:          doc.Notes,
	}
	return &metadata
}

// updateFromMetadata copies the corresponding data from the backups.Metadata
// into the backupMetadataDoc.
func (doc *backupMetadataDoc) updateFromMetadata(metadata *backups.Metadata) {
	// Ignore metadata.ID.
	doc.Started = metadata.Timestamp.Unix()
	doc.Finished = metadata.Finished.Unix()
	doc.CheckSum = metadata.CheckSum
	doc.CheckSumFormat = metadata.CheckSumFormat
	doc.Size = metadata.Size
	doc.Stored = metadata.Stored
	doc.Notes = metadata.Notes

	doc.Environment = metadata.Origin.Environment
	doc.Machine = metadata.Origin.Machine
	doc.Hostname = metadata.Origin.Hostname
	doc.Version = metadata.Origin.Version
}

//---------------------------
// DB operations

// getBackupMetadata returns the backup metadata associated with "id".
// If "id" does not match any stored records, an error satisfying
// juju/errors.IsNotFound() is returned.
func getBackupMetadata(st *State, id string) (*backups.Metadata, error) {
	collection, closer := st.getCollection(backupsC)
	defer closer()

	var doc backupMetadataDoc
	// There can only be one!
	err := collection.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("backup metadata %q", id)
	} else if err != nil {
		return nil, errors.Annotate(err, "error getting backup metadata")
	}

	return doc.asMetadata(), nil
}

// addBackupMetadata stores metadata for a backup where it can be
// accessed later.  It returns a new ID that is associated with the
// backup.  If the provided metadata already has an ID set, it is
// ignored.
func addBackupMetadata(st *State, metadata *backups.Metadata) (string, error) {
	// We use our own mongo _id value since the auto-generated one from
	// mongo may contain sensitive data (see bson.ObjectID).
	id, err := utils.NewUUID()
	if err != nil {
		return "", errors.Annotate(err, "error generating new ID")
	}
	idStr := id.String()
	return idStr, addBackupMetadataID(st, metadata, idStr)
}

func addBackupMetadataID(st *State, metadata *backups.Metadata, id string) error {
	var doc backupMetadataDoc
	doc.updateFromMetadata(metadata)
	doc.ID = id

	ops := []txn.Op{{
		C:      backupsC,
		Id:     doc.ID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
	if err := st.runTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			return errors.AlreadyExistsf("backup metadata %q", doc.ID)
		}
		return errors.Annotate(err, "error running transaction")
	}

	return nil
}

// setBackupStored updates the backup metadata associated with "id"
// to indicate that a backup archive has been stored.  If "id" does
// not match any stored records, an error satisfying
// juju/errors.IsNotFound() is returned.
func setBackupStored(st *State, id string) error {
	ops := []txn.Op{{
		C:      backupsC,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"stored", true},
		}}},
	}}
	if err := st.runTransaction(ops); err != nil {
		if err == txn.ErrAborted {
			return errors.NotFoundf(id)
		}
		return errors.Annotate(err, "error running transaction")
	}
	return nil
}
