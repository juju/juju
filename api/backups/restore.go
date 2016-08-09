// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
)

// TODO: There are no unit tests for this file.
// lp1545568 opened to track their addition.

var (
	// restoreStrategy is the attempt strategy for api server calls re-attempts in case
	// the server is upgrading.
	//
	// TODO(katco): 2016-08-09: lp:1611427
	restoreStrategy = utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Min:   1,
	}
)

// ClientConnection type represents a function capable of spawning a new Client connection
// it is used to pass around connection factories when necessary.
// TODO(perrito666) This is a workaround for lp:1399722 .
type ClientConnection func() (*Client, error)

// closerfunc is a function that allows you to close a client connection.
type closerFunc func() error

func prepareAttempt(client *Client) (error, error) {
	var remoteError error
	defer client.Close()
	err := client.facade.FacadeCall("PrepareRestore", nil, &remoteError)
	return err, remoteError
}

func prepareRestore(newClient ClientConnection) error {
	var err, remoteError error

	// PrepareRestore puts the server into a state that only allows
	// for restore to be called. This is to avoid the data loss if
	// users try to perform actions that are going to be overwritten
	// by restore.
	for a := restoreStrategy.Start(); a.Next(); {
		logger.Debugf("Will attempt to call 'PrepareRestore'")
		client, clientErr := newClient()
		if clientErr != nil {
			return errors.Trace(clientErr)
		}
		err, remoteError = prepareAttempt(client)
		if err == nil && remoteError == nil {
			return nil
		}
		if !params.IsCodeUpgradeInProgress(err) || remoteError != nil {
			return errors.Annotatef(err, "could not start prepare restore mode, server returned: %v", remoteError)
		}
	}
	return errors.Annotatef(err, "could not start restore process: %v", remoteError)
}

// RestoreReader restores the contents of backupFile as backup.
func (c *Client) RestoreReader(r io.ReadSeeker, meta *params.BackupsMetadataResult, newClient ClientConnection) error {
	if err := prepareRestore(newClient); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("Server is now in 'about to restore' mode, proceeding to upload the backup file")

	// Upload.
	backupId, err := c.Upload(r, *meta)
	if err != nil {
		finishErr := finishRestore(newClient)
		logger.Errorf("could not clean up after failed backup upload: %v", finishErr)
		return errors.Annotatef(err, "cannot upload backup file")
	}
	return c.restore(backupId, newClient)
}

// Restore performs restore using a backup id corresponding to a backup stored in the server.
func (c *Client) Restore(backupId string, newClient ClientConnection) error {
	if err := prepareRestore(newClient); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("Server in 'about to restore' mode")
	return c.restore(backupId, newClient)
}

func restoreAttempt(client *Client, restoreArgs params.RestoreArgs) (error, error) {
	var remoteError error
	defer client.Close()
	err := client.facade.FacadeCall("Restore", restoreArgs, &remoteError)
	return err, remoteError
}

// restore is responsible for triggering the whole restore process in a remote
// machine. The backup information for the process should already be in the
// server and loaded in the backup storage under the backupId id.
// It takes backupId as the identifier for the remote backup file and a
// client connection factory newClient (newClient should no longer be
// necessary when lp:1399722 is sorted out).
func (c *Client) restore(backupId string, newClient ClientConnection) error {
	var err, remoteError error

	// Restore
	restoreArgs := params.RestoreArgs{
		BackupId: backupId,
	}

	cleanExit := false
	for a := restoreStrategy.Start(); a.Next(); {
		logger.Debugf("Attempting Restore of %q", backupId)
		var restoreClient *Client
		restoreClient, err = newClient()
		if err != nil {
			return errors.Trace(err)
		}

		err, remoteError = restoreAttempt(restoreClient, restoreArgs)

		// A ShutdownErr signals that Restore almost certainly finished and
		// triggered Exit.
		if (err == nil || rpc.IsShutdownErr(err)) && remoteError == nil {
			cleanExit = true
			break
		}
		if !params.IsCodeUpgradeInProgress(err) || remoteError != nil {
			finishErr := finishRestore(newClient)
			logger.Errorf("could not clean up after failed restore attempt: %v", finishErr)
			return errors.Annotatef(err, "cannot perform restore: %v", remoteError)
		}
	}
	if !cleanExit {
		finishErr := finishRestore(newClient)
		if finishErr != nil {
			logger.Errorf("could not clean up failed restore: %v", finishErr)
		}
		return errors.Annotatef(err, "cannot perform restore: %v", remoteError)
	}

	err = finishRestore(newClient)
	if err != nil {
		return errors.Annotatef(err, "could not finish restore process: %v", remoteError)
	}
	return nil
}

func finishAttempt(client *Client) (error, error) {
	var remoteError error
	defer client.Close()
	err := client.facade.FacadeCall("FinishRestore", nil, &remoteError)
	return err, remoteError
}

// finishRestore since Restore call will end up with a reset
// controller, finish restore will check that the the newly
// placed controller has the mark of restore complete.
// upstart should have restarted the api server so we reconnect.
func finishRestore(newClient ClientConnection) error {
	var err, remoteError error
	for a := restoreStrategy.Start(); a.Next(); {
		logger.Debugf("Attempting finishRestore")
		var finishClient *Client
		finishClient, err = newClient()
		if err != nil {
			return errors.Trace(err)
		}

		err, remoteError = finishAttempt(finishClient)
		if err == nil && remoteError == nil {
			return nil
		}

		if !params.IsCodeUpgradeInProgress(err) || remoteError != nil {
			return errors.Annotatef(err, "cannot complete restore: %v", remoteError)
		}
	}
	return errors.Annotatef(err, "cannot complete restore: %v", remoteError)
}
