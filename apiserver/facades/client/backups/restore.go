// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"os"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

var bootstrapNode = names.NewMachineTag("0")

// Restore implements the server side of Backups.Restore.
func (a *API) Restore(p params.RestoreArgs) error {
	logger.Infof("Starting server side restore")

	// Get hold of a backup file Reader
	backup, closer := newBackups(a.backend)
	defer closer.Close()

	// Obtain the address of current machine, where we will be performing restore.
	machine, err := a.backend.Machine(a.machineID)
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

	info := a.backend.RestoreInfo()
	// Signal to current state and api server that restore will begin
	err = info.SetStatus(state.RestoreInProgress)
	if err != nil {
		return errors.Annotatef(err, "cannot set the server to %q mode", state.RestoreInProgress)
	}
	// Any abnormal termination of this function will mark restore as failed,
	// successful termination will call Exit and never run this.
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

	session := a.backend.MongoSession().Copy()
	defer session.Close()

	// Don't go if HA isn't ready.
	err = waitUntilReady(session, 60)
	if err != nil {
		return errors.Annotatef(err, "HA not ready; try again later")
	}

	oldTagString, err := backup.Restore(p.BackupId, restoreArgs)
	if err != nil {
		return errors.Annotate(err, "restore failed")
	}

	// A backup can be made of any component of an ha array.
	// The files in a backup don't contain purely relativized paths.
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

	// NOTE(fwereade): the apiserver needs to be restarted, yes, but
	// this approach is completely broken. The only place it's ever
	// ok to use os.Exit is in a main() func that's *so* simple as to
	// be reasonably left untested.
	//
	// And passing os.Exit in wouldn't make this any better either,
	// just using it subverts the expectations of *everything* else
	// running in the process.
	// XXX: We have ErrRestartAgent which should be capable of replacing this
	os.Exit(1)
	return nil
}

// PrepareRestore implements the server side of Backups.PrepareRestore.
func (a *API) PrepareRestore() error {
	info := a.backend.RestoreInfo()
	logger.Infof("entering restore preparation mode")
	return info.SetStatus(state.RestorePending)
}

// FinishRestore implements the server side of Backups.FinishRestore.
func (a *API) FinishRestore() error {
	info := a.backend.RestoreInfo()
	currentStatus, err := info.Status()
	if err != nil {
		return errors.Trace(err)
	}
	if currentStatus != state.RestoreFinished {
		if err := info.SetStatus(state.RestoreFailed); err != nil {
			return errors.Trace(err)
		}
		return errors.Errorf("Restore did not finish successfully")
	}
	logger.Infof("Successfully restored")
	return info.SetStatus(state.RestoreChecked)
}
