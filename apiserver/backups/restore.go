// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/restore"
)

// Restore implements the server side of Client.Restore
func (a *API) Restore(p params.Restore) error {
	if p.BackupId != "" {
		return fmt.Errorf("Backup from backups list not implemented")
	}
	filename := p.FileName
	filename = "/home/ubuntu/" + filename
	machine, err := a.st.Machine(p.Machine)
	if err != nil {
		return errors.Trace(err)
	}
	addr := network.SelectInternalAddress(machine.Addresses(), false)
	if addr == "" {
		return errors.Errorf("machine %q has no internal address", machine)
	}

	rInfo, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	err = rInfo.SetStatus(state.RestoreInProgress)


	if err := restore.Restore(filename, addr, a.st); err != nil {
		return errors.Annotate(err, "restore failed")
	}
	
	os.Exit(1)
	return nil

}

func (a *API) PrepareRestore() error {
	rInfo, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	err = rInfo.SetStatus(state.RestorePending)
	return errors.Annotatef(err, "cannot set restore status to %s", state.RestorePending)
}

func (a *API) FinishRestore() error {
	rInfo, err := a.st.EnsureRestoreInfo()
	if err != nil {
		return errors.Trace(err)
	}
	currentStatus := rInfo.Status()
	if currentStatus != state.RestoreFinished{
		return errors.Errorf("Restore did not finish succesfuly")
	}
	return nil
}
