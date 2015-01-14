// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// Restore implements the server side of Backups.Restore.
func (a *API) Restore(p params.RestoreAPIArgs) error {

	// Get hold of a backup file Reader
	backup, closer := newBackups(a.st)
	defer closer.Close()

	// Obtain the address of the machine where restore is going to be performed
	machine, err := a.st.Machine(p.Machine)
	if err != nil {
		return errors.Trace(err)
	}

	addr := network.SelectInternalAddress(machine.Addresses(), false)
	if addr == "" {
		return errors.Errorf("machine %q has no internal address", machine.Id())
	}

	// Signal to current state and api server that restore will begin
	info, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}

	err = info.SetStatus(state.RestoreInProgress)
	if err != nil {
		return errors.Annotatef(err, "cannot set the server to %q mode", state.RestoreInProgress)
	}

	instanceId, err := machine.InstanceId()
	if err != nil {
		return errors.Annotate(err, "cannot obtain instance id for machine to be restored")
	}

	logger.Infof("beginning server side restore of backup %q", p.BackupId)
	// Restore
	restoreArgs := params.RestoreArgs{
		PrivateAddress: addr,
		NewInstId:      instanceId,
		NewInstTag:     machine.Tag(),
		NewInstSeries:  machine.Series(),
	}
	if err := backup.Restore(p.BackupId, restoreArgs); err != nil {
		return errors.Annotate(err, "restore failed")
	}

	// After restoring, the api server needs a forced restart, tomb will not work
	// Exiting with 0 would prevent upstart to respawn the process
	os.Exit(1)
	return nil
}

// PrepareRestore implements the server side of Backups.PrepareRestore.
func (a *API) PrepareRestore() error {
	info, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("Entering about to restore mode")
	return info.SetStatus(state.RestorePending)
}

// FinishRestore implements the server side of Backups.FinishRestore.
func (a *API) FinishRestore() error {
	info, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	currentStatus := info.Status()
	if currentStatus != state.RestoreFinished {
		return errors.Errorf("Restore did not finish succesfuly")
	}
	logger.Infof("Succesfully restored")
	return info.SetStatus(state.RestoreChecked)
}
