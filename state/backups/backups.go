// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// backups contains all the stand-alone backup-related functionality for
// juju state.
package backups

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/filestorage"
	"github.com/juju/utils/tar"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/files"
	"github.com/juju/juju/state/backups/metadata"
)

var logger = loggo.GetLogger("juju.state.backups")

var (
	getFilesToBackUp = files.GetFilesToBackUp
	getDBDumper      = db.NewDumper
	runCreate        = create
	finishMeta       = func(meta *metadata.Metadata, result *createResult) error {
		return meta.Finish(result.size, result.checksum)
	}
	storeArchive = StoreArchive
)

// StoreArchive sends the backup archive and its metadata to storage.
// It also sets the metadata's ID and Stored values.
func StoreArchive(stor filestorage.FileStorage, meta *metadata.Metadata, file io.Reader) error {
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
	// Create creates and stores a new juju backup archive and returns
	// its associated metadata.
	Create(paths files.Paths, dbInfo db.Info, origin metadata.Origin, notes string) (*metadata.Metadata, error)
	// Get returns the metadata and archive file associated with the ID.
	Get(id string) (*metadata.Metadata, io.ReadCloser, error)
	// List returns the metadata for all stored backups.
	List() ([]metadata.Metadata, error)
	// Remove deletes the backup from storage.
	Remove(id string) error
	// Restore updates juju's state to the contents of the backup archive.
	Restore(io.ReadCloser, *metadata.Metadata, string, instance.Id) error
}

type backups struct {
	storage filestorage.FileStorage
}

// NewBackups returns a new backups based on the state.
func NewBackupsFromState(st *state.State) (Backups, io.Closer) {
	stor := state.NewBackupStorage(st)

	backups := NewBackups(stor)
	return backups, stor
}

// NewBackups returns a new Backups value using the provided DB info and
// file storage.
func NewBackups(stor filestorage.FileStorage) Backups {
	b := backups{
		storage: stor,
	}
	return &b
}

// Create creates and stores a new juju backup archive and returns
// its associated metadata.
func (b *backups) Create(paths files.Paths, dbInfo db.Info, origin metadata.Origin, notes string) (*metadata.Metadata, error) {

	// Prep the metadata.
	meta := metadata.NewMetadata(origin, notes, nil)
	// The metadata file will not contain the ID or the "finished" data.
	// However, that information is not as critical.  The alternatives
	// are either adding the metadata file to the archive after the fact
	// or adding placeholders here for the finished data and filling
	// them in afterward.  Neither is particularly trivial.
	metadataFile, err := meta.AsJSONBuffer()
	if err != nil {
		return nil, errors.Annotate(err, "while preparing the metadata")
	}

	// Create the archive.
	filesToBackUp, err := getFilesToBackUp("", paths)
	if err != nil {
		return nil, errors.Annotate(err, "while listing files to back up")
	}
	dumper, err := getDBDumper(dbInfo)
	if err != nil {
		return nil, errors.Annotate(err, "while preparing for DB dump")
	}
	args := createArgs{filesToBackUp, dumper, metadataFile}
	result, err := runCreate(&args)
	if err != nil {
		return nil, errors.Annotate(err, "while creating backup archive")
	}
	defer result.archiveFile.Close()

	// Finalize the metadata.
	err = finishMeta(meta, result)
	if err != nil {
		return nil, errors.Annotate(err, "while updating metadata")
	}

	// Store the archive.
	err = storeArchive(b.storage, meta, result.archiveFile)
	if err != nil {
		return nil, errors.Annotate(err, "while storing backup archive")
	}

	return meta, nil
}

// Get returns the metadata and archive file associated with the ID.
func (b *backups) Get(id string) (*metadata.Metadata, io.ReadCloser, error) {
	rawmeta, archiveFile, err := b.storage.Get(id)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	meta, ok := rawmeta.(*metadata.Metadata)
	if !ok {
		return nil, nil, errors.New("did not get a backups.Metadata value from storage")
	}

	return meta, archiveFile, nil
}

// List returns the metadata for all stored backups.
func (b *backups) List() ([]metadata.Metadata, error) {
	metaList, err := b.storage.List()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]metadata.Metadata, len(metaList))
	for i, meta := range metaList {
		m, ok := meta.(*metadata.Metadata)
		if !ok {
			msg := "expected backups.Metadata value from storage for %q, got %T"
			return nil, errors.Errorf(msg, meta.ID(), meta)
		}
		result[i] = *m
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
func (b *backups) Restore(backupFile io.ReadCloser, backupMetadata *metadata.Metadata, privateAddress string, newInstId instance.Id) error {
	workDir, err := ioutil.TempDir("", "juju-backup")
	if err != nil {
		return errors.Annotate(err, "cannot create temp folder")
	}
	defer os.Remove(workDir)

	// Extract outer container (a juju-backup folder inside the tar)
	tarFile, err := gzip.NewReader(backupFile)
	if err != nil {
		return errors.Trace(err)
	}
	if err := tar.UntarFiles(tarFile, workDir); err != nil {
		return errors.Trace(err)
	}

	backupFilesPath := filepath.Join(workDir, "juju-backup")
	// just in case, we dont want to leave these sensitive files hanging around.

	version, err := backupVersion(backupMetadata, backupFilesPath)
	if err != nil {
		return errors.Errorf("this is not a valid backup file")
	}

	// Extract inner container (root.tar inside the juju-backup)
	innerBackup := filepath.Join(backupFilesPath, "root.tar")
	var innerBackupHandler io.ReadSeeker
	innerBackupHandler, err = os.Open(innerBackup)
	if err != nil {
		return errors.Annotatef(err, "cannot open the backups inner tar file %q", innerBackup)
	}

	// Load configuration values that are to remain
	// without these values, lets leave before doing any possible damage on the system
	_, agentConfFile, err := tar.FindFile(innerBackupHandler, "var/lib/juju/agents/machine-0/agent.conf")
	if err != nil {
		return errors.Annotatef(err, "could not find agent configuration in tar file")
	}

	var agentConfig agent.ConfigSetterWriter
	agentConfig, err = agent.ParseConfig(agentConfFile, nil, "")
	if err != nil {
		return errors.Annotate(err, "cannot obtain agent configuration information")
	}

	// delete all the files to be replaced
	if err := files.PrepareMachineForRestore(); err != nil {
		return errors.Annotate(err, "cannot delete existing files")
	}

	// Reset cursor to 0 since tar wont do this
	innerBackupHandler.Seek(0, 0)

	if err := tar.UntarFiles(innerBackupHandler, filesystemRoot()); err != nil {
		return errors.Annotate(err, "cannot obtain system files from backup")
	}
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
	mongoDump := filepath.Join(backupFilesPath, "dump")
	if err := db.PlaceNewMongo(mongoDump, version); err != nil {
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
