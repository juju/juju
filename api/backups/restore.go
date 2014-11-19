// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
)

// Restore is responsable for finishing a restore after a placeholder
// machine has been bootstraped, it receives the name of a backup
// file on server and will return error on failure.
func (c *Client) Restore(backupFileName, backupId string, apiRoot func() (*api.State, error)) error {
	restoreArgs := params.RestoreArgs{
		FileName: backupFileName,
		BackupId: backupId,
		Machine:  "0",
	}
	// We will want to retry until upgrade is finished
	strategy := utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Min: 10,
	}
	var rErr error
	var err error
	for a := strategy.Start(); a.Next(); {
		err = c.facade.FacadeCall("Restore", restoreArgs, &rErr)
		// This signals that Restore almost certainly finished and
		// triggered Exit
		if err == rpc.ErrShutdown {
			break
		}
		if !params.IsCodeUpgradeInProgress(rErr) {
			return errors.Trace(rErr)
		}
	}
	if err != rpc.ErrShutdown {
		return errors.Annotate(err, "cannot perform restore")
	}
	// upstart should have restarted the api server so we reconnect
	root, err := apiRoot()
	if err != nil {
		return errors.Trace(err)
	}
	client := NewClient(root)
	defer client.Close()

	// FinishRestore since Restore call will end up with a reset
	// state server, finish restore will check that the the newly
	// placed state server has the mark of restore complete
	for a := strategy.Start(); a.Next(); {
		if err := client.facade.FacadeCall("FinishRestore", nil, &rErr); err == nil {
			break
		}
		if !params.IsCodeUpgradeInProgress(rErr) {
			return errors.Trace(rErr)
		}
	}
	return nil
}

// PrepareRestore puts the server into a state that only allows
// for restore to be called. This is to avoid the data loss if
// users try to perform actions that are going to be overwritten
// by restore.
func (c *Client) PrepareRestore() error {
	var err error
	var rErr error
	strategy := utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Min: 10,
	}
	for a := strategy.Start(); a.Next(); {
		err = c.facade.FacadeCall("PrepareRestore", nil, &rErr)
		if err != nil && !params.IsCodeUpgradeInProgress(rErr) {
			return errors.Trace(err)
		}
	}
	return errors.Annotate(rErr, "could not start restore process")
}
