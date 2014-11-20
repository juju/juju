// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
)

var logger = loggo.GetLogger("juju.api.backups")

type ClientConnection func() (*Client, func() error, error)

// Restore is responsable for finishing a restore after a placeholder
// machine has been bootstraped, it receives the name of a backup
// file on server and will return error on failure.
func (c *Client) Restore(backupFileName, backupId string, newClient ClientConnection) error {
	var (
		err  error
		rErr error
	)
	strategy := utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Min:   10,
	}

	// PrepareRestore puts the server into a state that only allows
	// for restore to be called. This is to avoid the data loss if
	// users try to perform actions that are going to be overwritten
	// by restore.
	for a := strategy.Start(); a.Next(); {
		logger.Debugf("Will attemt to call 'PrepareRestore'")
		prepareClient, prepareClientCloser, err := newClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer prepareClientCloser()
		if err = prepareClient.facade.FacadeCall("PrepareRestore", nil, &rErr); err == nil {
			break
		}
		if err != nil && !params.IsCodeUpgradeInProgress(rErr) {
			return errors.Trace(err)
		}
	}
	if err != nil {
		return errors.Annotatef(err, "could not start restore process: %v", rErr)
	}
	logger.Debugf("Server in 'about to restore' mode")

	// Upload
	if backupFileName != "" {
		backupFile, err := os.Open(backupFileName)
		if err != nil {
			return errors.Annotatef(err, "cannot open backup file %q", backupFileName)
		}
		logger.Debugf("Uploading %q", backupFileName)
		if backupId, err = c.Upload(backupFile); err != nil {
			return errors.Annotatef(err, "cannot upload backup file %s", backupFileName)
		}
	}

	// Restore
	restoreArgs := params.RestoreArgs{
		BackupId: backupId,
		Machine:  "0",
	}

	for a := strategy.Start(); a.Next(); {
		logger.Debugf("Attempting Restore")
		restoreClient, restoreClientCloser, err := newClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer restoreClientCloser()

		err = restoreClient.facade.FacadeCall("Restore", restoreArgs, &rErr)

		// This signals that Restore almost certainly finished and
		// triggered Exit.
		if err == rpc.ErrShutdown && rErr == nil {
			break
		}
		if err != nil && !params.IsCodeUpgradeInProgress(rErr) {
			return errors.Annotatef(err, "cannot perform restore: %v", rErr)
		}
	}
	if err != rpc.ErrShutdown {
		return errors.Annotatef(err, "cannot perform restore: %v", rErr)
	}

	// FinishRestore since Restore call will end up with a reset
	// state server, finish restore will check that the the newly
	// placed state server has the mark of restore complete.
	// upstart should have restarted the api server so we reconnect.
	for a := strategy.Start(); a.Next(); {
		logger.Debugf("Attempting FinishRestore")
		finishClient, finishClientCloser, err := newClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer finishClientCloser()

		if err = finishClient.facade.FacadeCall("FinishRestore", nil, &rErr); err == nil {
			break
		}
		if err != nil && !params.IsCodeUpgradeInProgress(rErr) {
			return errors.Annotatef(err, "cannot complete restore: %v", rErr)
		}
	}
	if err != nil {
		return errors.Annotatef(err, "could not finish restore process: %v", rErr)
	}
	return nil
}
