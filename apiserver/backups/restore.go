// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

// Restore implements the server side of Backups.Restore.
func (a *API) Restore(p params.RestoreArgs) error {

	// Get hold of a backup file Reader
	backup, closer := newBackups(a.st)
	defer closer.Close()

	// Obtain the address of current machine, where we will be performing restore.
	machine, err := a.st.Machine(a.machineID)
	if err != nil {
		return errors.Trace(err)
	}

	addr := network.SelectInternalAddress(machine.Addresses(), false)
	if addr == "" {
		return errors.Errorf("machine %q has no internal address", machine)
	}

	publicAddress := network.SelectPublicAddress(machine.Addresses())
	if publicAddress == "" {
		return errors.Errorf("machine %q has no public address", machine)
	}

	info, err := a.st.RestoreInfoSetter()
	if err != nil {
		return errors.Trace(err)
	}
	// Signal to current state and api server that restore will begin
	err = info.SetStatus(state.RestoreInProgress)
	if err != nil {
		return errors.Annotatef(err, "cannot set the server to %q mode", state.RestoreInProgress)
	}
	// Any abnormal termination of this function will mark restore as failed,
	// succesful termination will call Exit and never run this.
	defer info.SetStatus(state.RestoreFailed)

	instanceId, err := machine.InstanceId()
	if err != nil {
		return errors.Annotate(err, "cannot obtain instance id for machine to be restored")
	}

	logger.Infof("beginning server side restore of backup %q", p.BackupId)
	// Restore
	restoreArgs := backups.RestoreArgs{
		PrivateAddress: addr,
		PublicAddress:  publicAddress,
		NewInstId:      instanceId,
		NewInstTag:     machine.Tag(),
		NewInstSeries:  machine.Series(),
	}
	if err := backup.Restore(p.BackupId, restoreArgs); err != nil {
		return errors.Annotate(err, "restore failed")
	}

	// After restoring, the api server needs a forced restart, tomb will not work
	// this is because we change all of juju configuration files and mongo too.
	// Exiting with 0 would prevent upstart to respawn the process
	os.Exit(1)
	return nil
}

// PrepareRestore implements the server side of Backups.PrepareRestore.
func (a *API) PrepareRestore() error {
	info, err := a.st.RestoreInfoSetter()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Infof("entering restore preparation mode")
	return info.SetStatus(state.RestorePending)
}

// FinishRestore implements the server side of Backups.FinishRestore.
func (a *API) FinishRestore() error {
	info, err := a.st.RestoreInfoSetter()
	if err != nil {
		return errors.Trace(err)
	}
	currentStatus := info.Status()
	if currentStatus != state.RestoreFinished {
		info.SetStatus(state.RestoreFailed)
		return errors.Errorf("Restore did not finish succesfuly")
	}
	logger.Infof("Succesfully restored")
	return info.SetStatus(state.RestoreChecked)
}
