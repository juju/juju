// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/metadata"
)

const restoreUserHome = "/home/ubuntu/"

func (a *API) backupFileAndMeta(backupId, fileName string, backup backups.Backups) (io.ReadCloser, *metadata.Metadata, error) {
	var (
		fileHandler io.ReadCloser
		meta        *metadata.Metadata
		err         error
	)
	switch {
	case backupId != "":
		if meta, fileHandler, err = backup.Get(backupId); err != nil {
			return nil, nil, errors.Annotatef(err, "could not fetch backup %q", backupId)
		}
	case fileName != "":
		fileName := restoreUserHome + fileName
		if fileHandler, err = os.Open(fileName); err != nil {
			return nil, nil, errors.Annotatef(err, "error opening %q", fileName)
		}
	default:
		return nil, nil, errors.Errorf("no backup file or id given")

	}
	return fileHandler, meta, nil

}

// Restore implements the server side of Backups.Restore.
func (a *API) Restore(p params.RestoreArgs) error {
	// Get hold of a backup file Reader
	backup, closer := newBackups(a.st)
	defer closer.Close()

	fileHandler, meta, err := a.backupFileAndMeta(p.BackupId, p.FileName, backup)
	if err != nil {
		return errors.Annotate(err, "cannot obtain a backup")
	}
	defer fileHandler.Close()

	// Obtain the address of the machine where restore is going to be performed
	machine, err := a.st.Machine(p.Machine)
	if err != nil {
		return errors.Trace(err)
	}
	addr := network.SelectInternalAddress(machine.Addresses(), false)
	if addr == "" {
		return errors.Errorf("machine %q has no internal address", machine)
	}

	// Signal to current state and api server that restore will begin
	rInfo, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	err = rInfo.SetStatus(state.RestoreInProgress)

	instanceId, err := machine.InstanceId()
	if err != nil {
		return errors.Annotate(err, "cannot obtain instance id for machine to be restored")
	}

	// Restore
	if err := backup.Restore(fileHandler, meta, addr, instanceId); err != nil {
		return errors.Annotate(err, "restore failed")
	}

	// After restoring, the api server needs a forced restart, tomb will not work
	// Exiting with 0 would prevent upstart to respawn the process
	os.Exit(1)
	return nil
}

// PrepareRestore implements the server side of Backups.PrepareRestore.
func (a *API) PrepareRestore() error {
	rInfo, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	return rInfo.SetStatus(state.RestorePending)
}

// FinishRestore implements the server side of Backups.FinishRestore.
func (a *API) FinishRestore() error {
	rInfo, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	currentStatus := rInfo.Status()
	if currentStatus != state.RestoreFinished {
		return errors.Errorf("Restore did not finish succesfuly")
	}
	return rInfo.SetStatus(state.RestoreChecked)
}
