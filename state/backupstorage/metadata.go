// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/version"
)

// NewBackupsOrigin returns a snapshot of where backup was run.  That
// snapshot is a new backup Origin value, for use in a backup's
// metadata.  Every value except for the machine name is populated
// either from juju state or some other implicit mechanism.
func NewBackupsOrigin(st *state.State, machine string) *metadata.Origin {
	// hostname could be derived from the environment...
	hostname, err := os.Hostname()
	if err != nil {
		// If os.Hostname() is not working, something is woefully wrong.
		// Run for the hills.
		panic(fmt.Sprintf("could not get hostname (system unstable?): %v", err))
	}
	origin := metadata.NewOrigin(
		st.EnvironTag().Id(),
		machine,
		hostname,
	)
	return origin
}

// metadataDoc is a mirror of metadata.Metadata, used just for DB storage.
type metadataDoc struct {
	ID             string `bson:"_id"`
	Started        int64  `bson:"started,minsize"`
	Finished       int64  `bson:"finished,minsize"`
	Checksum       string `bson:"checksum"`
	ChecksumFormat string `bson:"checksumformat"`
	Size           int64  `bson:"size,minsize"`
	Stored         bool   `bson:"stored"`
	Notes          string `bson:"notes,omitempty"`

	// origin
	Environment string         `bson:"environment"`
	Machine     string         `bson:"machine"`
	Hostname    string         `bson:"hostname"`
	Version     version.Number `bson:"version"`
}

func (doc *metadataDoc) fileSet() bool {
	if doc.Finished == 0 {
		return false
	}
	if doc.Checksum == "" {
		return false
	}
	if doc.ChecksumFormat == "" {
		return false
	}
	if doc.Size == 0 {
		return false
	}
	return true
}

func (doc *metadataDoc) validate() error {
	if doc.ID == "" {
		return errors.New("missing ID")
	}
	if doc.Started == 0 {
		return errors.New("missing Started")
	}
	if doc.Environment == "" {
		return errors.New("missing Environment")
	}
	if doc.Machine == "" {
		return errors.New("missing Machine")
	}
	if doc.Hostname == "" {
		return errors.New("missing Hostname")
	}
	if doc.Version.Major == 0 {
		return errors.New("missing Version")
	}

	// Check the file-related fields.
	if !doc.fileSet() {
		if doc.Stored {
			return errors.New(`"Stored" flag is unexpectedly true`)
		}
		// Don't check the file-related fields.
		return nil
	}
	if doc.Finished == 0 {
		return errors.New("missing Finished")
	}
	if doc.Checksum == "" {
		return errors.New("missing Checksum")
	}
	if doc.ChecksumFormat == "" {
		return errors.New("missing ChecksumFormat")
	}
	if doc.Size == 0 {
		return errors.New("missing Size")
	}

	return nil
}

// asMetadata returns a new metadata.Metadata based on the metadataDoc.
func (doc *metadataDoc) asMetadata() *metadata.Metadata {
	// Create a new Metadata.
	origin := metadata.ExistingOrigin(
		doc.Environment,
		doc.Machine,
		doc.Hostname,
		doc.Version,
	)

	started := time.Unix(doc.Started, 0).UTC()
	meta := metadata.NewMetadata(
		*origin,
		doc.Notes,
		&started,
	)

	// The ID is already set.
	meta.SetID(doc.ID)

	// Exit early if file-related fields not set.
	if !doc.fileSet() {
		return meta
	}

	// Set the file-related fields.
	var finished *time.Time
	if doc.Finished != 0 {
		val := time.Unix(doc.Finished, 0).UTC()
		finished = &val
	}
	err := meta.Finish(doc.Size, doc.Checksum, doc.ChecksumFormat, finished)
	if err != nil {
		// The doc should have already been validated.  An error here
		// indicates that Metadata changed and metadataDoc did not
		// accommodate the change.  Thus an error here indicates a
		// developer "error".  A caller should not need to worry about
		// that case so we panic instead of passing the error out.
		panic(fmt.Sprintf("unexpectedly invalid metadata doc: %v", err))
	}
	if doc.Stored {
		meta.SetStored()
	}
	return meta
}

// updateFromMetadata copies the corresponding data from the backup
// Metadata into the metadataDoc.
func (doc *metadataDoc) updateFromMetadata(metadata *metadata.Metadata) {
	finished := metadata.Finished()
	// Ignore metadata.ID.
	doc.Started = metadata.Started().Unix()
	if finished != nil {
		doc.Finished = finished.Unix()
	}
	doc.Checksum = metadata.Checksum()
	doc.ChecksumFormat = metadata.ChecksumFormat()
	doc.Size = metadata.Size()
	doc.Stored = metadata.Stored()
	doc.Notes = metadata.Notes()

	origin := metadata.Origin()
	doc.Environment = origin.Environment()
	doc.Machine = origin.Machine()
	doc.Hostname = origin.Hostname()
	doc.Version = origin.Version()
}
