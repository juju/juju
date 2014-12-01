// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
)

var logger = loggo.GetLogger("juju.api.backups")

//TODO(perrito666): this Key needs to be determined in a dinamic way since machine 0
// might not always be there in HA contexts
var (
	RestoreMachineKey = "0"
	RestoreStrategy   = utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Min:   10,
	}
)

type ClientConnection func() (*Client, func() error, error)

func prepareRestore(newClient ClientConnection) error {
	var (
		err  error
		rErr error
	)
	// PrepareRestore puts the server into a state that only allows
	// for restore to be called. This is to avoid the data loss if
	// users try to perform actions that are going to be overwritten
	// by restore.
	for a := RestoreStrategy.Start(); a.Next(); {
		logger.Debugf("Will attemt to call 'PrepareRestore'")
		prepareClient, prepareClientCloser, err := newClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer prepareClientCloser()
		err = prepareClient.facade.FacadeCall("PrepareRestore", nil, &rErr)
		if err == nil {
			break
		}
		if !params.IsCodeUpgradeInProgress(rErr) {
			return errors.Annotatef(err, "could not start prepare restore mode, server returned: %v", rErr)
		}
	}
	return errors.Annotatef(err, "could not start restore process: %v", rErr)
}

func (c *Client) RestoreFromFile(backupFile io.Reader, newClient ClientConnection) error {
	if err := prepareRestore(newClient); err != nil {
		errors.Trace(err)
	}
	logger.Debugf("Server in 'about to restore' mode")
	// Upload
	logger.Debugf("Uploading backup")
	backupId, err := c.Upload(backupFile)
	if err != nil {
		//TODO(perrito666): this is a recoverable error, we should undo Prepare
		return errors.Annotatef(err, "cannot upload backup file")
	}
	return c.restore(backupId, newClient)
}

func (c *Client) RestoreFromID(backupId string, newClient ClientConnection) error {
	if err := prepareRestore(newClient); err != nil {
		errors.Trace(err)
	}
	logger.Debugf("Server in 'about to restore' mode")
	return c.restore(backupId, newClient)
}

// Restore is responsable for finishing a restore after a placeholder
// machine has been bootstraped, it receives the name of a local backup file
// or the id of one in the server and will return error on failure.
func (c *Client) restore(backupId string, newClient ClientConnection) error {
	var (
		err  error
		rErr error
	)

	// Restore
	restoreArgs := params.RestoreArgs{
		BackupId: backupId,
		Machine:  RestoreMachineKey,
	}

	for a := RestoreStrategy.Start(); a.Next(); {
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
		//TODO(perrito666): There are some of the possible outcomes that might
		// deserve disable PrepareRestore mode
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
	for a := RestoreStrategy.Start(); a.Next(); {
		logger.Debugf("Attempting FinishRestore")
		finishClient, finishClientCloser, err := newClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer finishClientCloser()

		err = finishClient.facade.FacadeCall("FinishRestore", nil, &rErr)
		if err == nil {
			break
		}
		if !params.IsCodeUpgradeInProgress(rErr) {
			return errors.Annotatef(err, "cannot complete restore: %v", rErr)
		}
	}
	if err != nil {
		return errors.Annotatef(err, "could not finish restore process: %v", rErr)
	}
	return nil
}
