// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package backups contains all the stand-alone backup-related
functionality for juju state. That functionality is encapsulated by
the backups.Backups type. The package also exposes a few key helpers
and components.

Backups are not a part of juju state nor of normal state operations.
However, they certainly are tightly coupled with state (the very
subject of backups). This puts backups in an odd position, particularly
with regard to the storage of backup metadata and archives.

As noted above backups are about state but not a part of state. So
exposing backup-related methods on State would imply the wrong thing.
Thus most of the functionality here is defined at a high level without
relation to state. A few low-level parts or helpers are exposed as
functions to which you pass a state value. Those are kept to a minimum.

Note that state (and juju as a whole) currently does not have a
persistence layer abstraction to facilitate separating different
persistence needs and implementations. As a consequence, state's
data, whether about how an environment should look or about existing
resources within an environment, is dumped essentially straight into
State's mongo connection. The code in the state package does not
make any distinction between the two (nor does the package clearly
distinguish between state-related abstractions and state-related
data).

Backups add yet another category, merely taking advantage of State's
mongo for storage. In the interest of making the distinction clear,
among other reasons, backups uses its own database under state's mongo
connection.
*/
package backups

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/filestorage"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/db"
)

const (
	// FilenamePrefix is the prefix used for backup archive files.
	FilenamePrefix = "juju-backup-"

	// FilenameTemplate is used with time.Time.Format to generate a filename.
	FilenameTemplate = FilenamePrefix + "20060102-150405.tar.gz"
)

var logger = loggo.GetLogger("juju.state.backups")

var (
	getFilesToBackUp = GetFilesToBackUp
	getDBDumper      = NewDBDumper
	runCreate        = create
	finishMeta       = func(meta *Metadata, result *createResult) error {
		return meta.MarkComplete(result.size, result.checksum)
	}
	storeArchive = StoreArchive
)

// StoreArchive sends the backup archive and its metadata to storage.
// It also sets the metadata's ID and Stored values.
func StoreArchive(stor filestorage.FileStorage, meta *Metadata, file io.Reader) error {
	id, err := stor.Add(meta, file)
	if err != nil {
		return errors.Trace(err)
	}
	meta.SetID(id)
	stored, err := stor.Metadata(id)
	if err != nil {
		return errors.Trace(err)
	}
	meta.SetStored(stored.Stored())
	return nil
}

// Backups is an abstraction around all juju backup-related functionality.
type Backups interface {
	// Create creates and stores a new juju backup archive. It updates
	// the provided metadata.
	Create(meta *Metadata, paths *Paths, dbInfo *DBInfo) error

	// Get returns the metadata and archive file associated with the ID.
	Get(id string) (*Metadata, io.ReadCloser, error)

	// List returns the metadata for all stored backups.
	List() ([]*Metadata, error)

	// Remove deletes the backup from storage.
	Remove(id string) error
	// Restore updates juju's state to the contents of the backup archive.
	Restore(io.ReadCloser, string, instance.Id) error
}

type backups struct {
	storage filestorage.FileStorage
}

// NewBackups creates a new Backups value using the FileStorage provided.
func NewBackups(stor filestorage.FileStorage) Backups {
	b := backups{
		storage: stor,
	}
	return &b
}

// Create creates and stores a new juju backup archive and updates the
// provided metadata.
func (b *backups) Create(meta *Metadata, paths *Paths, dbInfo *DBInfo) error {
	meta.Started = time.Now().UTC()

	// The metadata file will not contain the ID or the "finished" data.
	// However, that information is not as critical. The alternatives
	// are either adding the metadata file to the archive after the fact
	// or adding placeholders here for the finished data and filling
	// them in afterward.  Neither is particularly trivial.
	metadataFile, err := meta.AsJSONBuffer()
	if err != nil {
		return errors.Annotate(err, "while preparing the metadata")
	}

	// Create the archive.
	filesToBackUp, err := getFilesToBackUp("", paths)
	if err != nil {
		return errors.Annotate(err, "while listing files to back up")
	}
	dumper, err := getDBDumper(dbInfo)
	if err != nil {
		return errors.Annotate(err, "while preparing for DB dump")
	}
	args := createArgs{filesToBackUp, dumper, metadataFile}
	result, err := runCreate(&args)
	if err != nil {
		return errors.Annotate(err, "while creating backup archive")
	}
	defer result.archiveFile.Close()

	// Finalize the metadata.
	err = finishMeta(meta, result)
	if err != nil {
		return errors.Annotate(err, "while updating metadata")
	}

	// Store the archive.
	err = storeArchive(b.storage, meta, result.archiveFile)
	if err != nil {
		return errors.Annotate(err, "while storing backup archive")
	}

	return nil
}

// Get pulls the associated metadata and archive file from environment storage.
func (b *backups) Get(id string) (*Metadata, io.ReadCloser, error) {
	if strings.HasPrefix(id, uploadedPrefix) {
		archiveFile, err := openUploaded(id)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		// TODO(ericsnow) Extract the metadata from the file.
		return nil, archiveFile, nil
	}

	rawmeta, archiveFile, err := b.storage.Get(id)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	meta, ok := rawmeta.(*Metadata)
	if !ok {
		return nil, nil, errors.New("did not get a backups.Metadata value from storage")
	}

	return meta, archiveFile, nil
}

// List returns the metadata for all stored backups.
func (b *backups) List() ([]*Metadata, error) {
	metaList, err := b.storage.List()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*Metadata, len(metaList))
	for i, meta := range metaList {
		m, ok := meta.(*Metadata)
		if !ok {
			msg := "expected backups.Metadata value from storage for %q, got %T"
			return nil, errors.Errorf(msg, meta.ID(), meta)
		}
		result[i] = m
	}
	return result, nil
}

// Remove deletes the backup from storage.
func (b *backups) Remove(id string) error {
	return errors.Trace(b.storage.Remove(id))
}

// Restore handles either returning or creating a state server to a backed up status:
// * extracts the content of the given backup file and:
// * runs mongorestore with the backed up mongo dump
// * updates and writes configuration files
// * updates existing db entries to make sure they hold no references to
// old instances
// * updates config in all agents.
func (b *backups) Restore(backupFile io.ReadCloser, privateAddress string, newInstId instance.Id) error {
	workspace, err := NewArchiveWorkspaceReader(backupFile)
	if err != nil {
		return errors.Annotate(err, "cannot unpack backup file")
	}
	defer workspace.Close()

	meta, err := workspace.Metadata()
	if err != nil {
		return errors.Annotatef(err, "cannot read metadata file, this backup is either too old or corrupt")
	}
	version := meta.Origin.Version

	// delete all the files to be replaced
	if err := PrepareMachineForRestore(); err != nil {
		return errors.Annotate(err, "cannot delete existing files")
	}

	if err := workspace.UnpackFilesBundle(filesystemRoot()); err != nil {
		return errors.Annotate(err, "cannot obtain system files from backup")
	}

	var agentConfig agent.ConfigSetterWriter
	if agentConfig, err = agent.ReadConfig("/var/lib/juju/agents/machine-0/agent.conf"); err != nil {
		return errors.Annotate(err, "cannot load agent config from disk")
	}
	ssi, ok := agentConfig.StateServingInfo()
	if !ok {
		return errors.Errorf("cannot determine state serving info")
	}

	APIHostPort := network.HostPort{Address: network.Address{
		Value: privateAddress,
		Type:  network.DeriveAddressType(privateAddress),
	},
		Port: ssi.APIPort}
	agentConfig.SetAPIHostPorts([][]network.HostPort{{APIHostPort}})
	if err := agentConfig.Write(); err != nil {
		return errors.Annotate(err, "cannot write new agent configuration")
	}

	// Restore backed up mongo
	if err := db.PlaceNewMongo(workspace.DBDumpDir, version); err != nil {
		return errors.Annotate(err, "error restoring state from backup")
	}

	// Re-start replicaset with the new value for server address
	dialInfo, err := newDialInfo(privateAddress, agentConfig)
	if err != nil {
		return errors.Annotate(err, "cannot produce dial information")
	}

	memberHostPort := fmt.Sprintf("%s:%d", privateAddress, ssi.StatePort)
	err = resetReplicaSet(dialInfo, memberHostPort)
	if err != nil {
		return errors.Annotate(err, "cannot reset replicaSet")
	}

	// Update entries for machine 0 to point to the newest instance
	err = updateMongoEntries(newInstId, dialInfo)
	if err != nil {
		return errors.Annotate(err, "cannot update mongo entries")
	}

	// From here we work with the restored state server
	st, err := newStateConnection(agentConfig)
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Close()

	// update all agents known to the new state server.
	// TODO(perrito666): We should never stop process because of this
	// it is too late to go back and errors in a couple of agents have
	// better change of being fixed by the user, if we where to fail
	// we risk an inconsistent state server because of one unresponsive
	// agent, we should nevertheless return the err info to the user.
	// for this updateAllMachines will not return errors for individual
	// agent update failures
	err = updateAllMachines(privateAddress, st)
	if err != nil {
		return errors.Annotate(err, "cannot update agents")
	}

	rInfo, err := st.EnsureRestoreInfo()

	if err != nil {
		return errors.Trace(err)
	}

	// Mark restoreInfo as Finished so upon restart of the apiserver
	// the client can reconnect and determine if we where succesful.
	err = rInfo.SetStatus(state.RestoreFinished)

	return errors.Annotate(err, "failed to set status to finished")
}

// The following code is temporary and in support of the initial
// restore patch.  Once we have an HTTP-based upload this code will
// be removed.

const (
	uploadedPrefix = "file://"
	sshUsername    = "ubuntu"
)

type sendFunc func(host, filename string, archive io.Reader) error

// SimpleUpload sends the backup archive to the server where it is saved
// in the home directory of the SSH user.  The returned ID may be used
// to locate the file on the server.
func SimpleUpload(publicAddress string, archive io.Reader, send sendFunc) (string, error) {
	filename := time.Now().UTC().Format(FilenameTemplate)
	host := sshUsername + "@" + publicAddress
	err := send(host, filename, archive)
	return uploadedPrefix + filename, errors.Trace(err)
}

func resolveUploaded(id string) (string, error) {
	filename := strings.TrimPrefix(id, uploadedPrefix)
	filename = filepath.FromSlash(filename)
	if !strings.HasPrefix(filepath.Base(filename), FilenamePrefix) {
		return "", errors.Errorf("invalid ID for uploaded file: %q", id)
	}
	if filepath.IsAbs(filename) {
		return "", errors.Errorf("expected relative path in ID, got %q", id)
	}

	sshUser, err := user.Lookup(sshUsername)
	if err != nil {
		return "", errors.Trace(err)
	}
	filename = filepath.Join(sshUser.HomeDir, filename)
	return filename, nil
}

func openUploaded(id string) (io.ReadCloser, error) {
	filename, err := resolveUploaded(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	archive, err := os.Open(filename)
	return archive, errors.Trace(err)
}
