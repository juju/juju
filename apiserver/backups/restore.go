// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"fmt"
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
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

	if err := restore.Restore(filename, addr, a.st); err != nil {
		return err
	}
	os.Exit(1)
	return nil

}

func (a *API) PrepareRestore() error {
	return nil
}

func (a *API) FinishRestore() error {
	return nil
}
