// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"os"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

var bootstrapNode = names.NewMachineTag("0")

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

	addr, err := machine.PrivateAddress()
	if err != nil {
		return errors.Annotatef(err, "error fetching internal address for machine %q", machine)

	}

	publicAddress, err := machine.PublicAddress()
	if err != nil {
		return errors.Annotatef(err, "error fetching public address for machine %q", machine)

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
		PrivateAddress: addr.Value,
		PublicAddress:  publicAddress.Value,
		NewInstId:      instanceId,
		NewInstTag:     machine.Tag(),
		NewInstSeries:  machine.Series(),
	}

	oldTagString, err := backup.Restore(p.BackupId, restoreArgs)
	if err != nil {
		return errors.Annotate(err, "restore failed")
	}

	// A backup can be made of any component of an ha array.
	// The files in a backup dont contain purely relativized paths.
	// If the backup is made of the bootstrap node (machine 0) the
	// recently created machine will have the same paths and therefore
	// the startup scripts will fit the new juju. If the backup belongs
	// to a different machine, we need to create a new set of startup
	// scripts and exit with 0 (so that the current script does not try
	// to restart the old juju, which will no longer be there).
	if oldTagString != nil && oldTagString != bootstrapNode {
		srvName := fmt.Sprintf("jujud-%s", oldTagString)
		srv, err := service.DiscoverService(srvName, common.Conf{})
		if err != nil {
			return errors.Annotatef(err, "cannot find %q service", srvName)
		}
		if err := srv.Start(); err != nil {
			return errors.Annotatef(err, "cannot start %q service", srvName)
		}
		// We dont want machine-0 to restart since the new one has a different tag.
		// We started the new one above.
		os.Exit(0)
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
