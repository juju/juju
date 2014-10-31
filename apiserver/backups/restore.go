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
	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/state/restore"
)

const restoreUserHome = "/home/ubuntu/"

func backupFile(backupPath string) (io.ReadCloser, error) {
	return os.Open(backupPath)
}

// Restore implements the server side of Backups.Restore
func (a *API) Restore(p params.RestoreArgs) error {
	// Get hold of a backup file Reader
	var (
		fileHandler io.ReadCloser
		meta        *metadata.Metadata
		err         error
	)
	switch {
	case p.BackupId != "":
		if meta, fileHandler, err = a.backups.Get(p.BackupId); err != nil {
			return errors.Annotatef(err, "there was an error obtaining backup %q", p.BackupId)
		}
	case p.FileName != "":
		filename := restoreUserHome + p.FileName
		if fileHandler, err = backupFile(filename); err != nil {
			return errors.Annotatef(err, "there was an error opening %q", filename)
		}
	default:
		return errors.Errorf("no backup file or id given")

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

	// Restore
	if err := restore.Restore(fileHandler, meta, addr, a.st); err != nil {
		return errors.Annotate(err, "restore failed")
	}

	// After restoring, the api server needs a forced restart, tomb will not work
	if err == nil {
		os.Exit(1)
	}

	return errors.Annotate(err, "failed to restore")

}

// PrepareRestore implements the server side of Backups.PrepareRestore
func (a *API) PrepareRestore() error {
	rInfo, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	err = rInfo.SetStatus(state.RestorePending)
	return errors.Annotatef(err, "cannot set restore status to %s", state.RestorePending)
}

// FinishRestore implements the server side of Backups.FinishRestore
func (a *API) FinishRestore() error {
	rInfo, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	currentStatus := rInfo.Status()
	if currentStatus != state.RestoreFinished {
		return errors.Errorf("Restore did not finish succesfuly")
	}
	err = rInfo.SetStatus(state.RestoreChecked)
	return errors.Annotate(err, "could not mark restore as completed")
}
