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

var (
	// restoreStrategy is the attempt strategy for api server calls re-attempts in case
	// the server is upgrading.
	restoreStrategy = utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Min:   10,
	}
)

// ClientConnection type represents a function capable of spawning a new Client connection
// it is used to pass around connection factories when necessary.
// TODO(perrito666) This is a workaround for lp:1399722 .
type ClientConnection func() (*Client, func() error, error)

// closerfunc is a function that allows you to close a client connection.
type closerFunc func() error

func prepareAttempt(client *Client, closer closerFunc) (error, error) {
	var remoteError error
	defer closer()
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
		client, clientCloser, clientErr := newClient()
		if clientErr != nil {
			return errors.Trace(clientErr)
		}
		if err, remoteError = prepareAttempt(client, clientCloser); err == nil {
			return nil
		}
		if !params.IsCodeUpgradeInProgress(remoteError) {
			return errors.Annotatef(err, "could not start prepare restore mode, server returned: %v", remoteError)
		}
	}
	return errors.Annotatef(err, "could not start restore process: %v", remoteError)
}

// RestoreReader restores the contents of backupFile as backup.
func (c *Client) RestoreReader(r io.Reader, meta *params.BackupsMetadataResult, newClient ClientConnection) error {
	if err := prepareRestore(newClient); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("Server is now in 'about to restore' mode, proceeding to upload the backup file")

	// Upload.
	backupId, err := c.Upload(r, *meta)
	if err != nil {
		finishErr := finishRestore(newClient)
		logger.Errorf("could not exit restoring status: %v", finishErr)
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

func restoreAttempt(client *Client, closer closerFunc, restoreArgs params.RestoreArgs) (error, error) {
	var remoteError error
	defer closer()
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

	for a := restoreStrategy.Start(); a.Next(); {
		logger.Debugf("Attempting Restore of %q", backupId)
		restoreClient, restoreClientCloser, err := newClient()
		if err != nil {
			return errors.Trace(err)
		}

		err, remoteError = restoreAttempt(restoreClient, restoreClientCloser, restoreArgs)

		// This signals that Restore almost certainly finished and
		// triggered Exit.
		if err == rpc.ErrShutdown && remoteError == nil {
			break
		}
		if err != nil && !params.IsCodeUpgradeInProgress(remoteError) {
			finishErr := finishRestore(newClient)
			logger.Errorf("could not exit restoring status: %v", finishErr)
			return errors.Annotatef(err, "cannot perform restore: %v", remoteError)
		}
	}
	if err != rpc.ErrShutdown {
		finishErr := finishRestore(newClient)
		if finishErr != nil {
			logger.Errorf("could not exit restoring status: %v", finishErr)
		}
		return errors.Annotatef(err, "cannot perform restore: %v", remoteError)
	}

	err = finishRestore(newClient)
	if err != nil {
		return errors.Annotatef(err, "could not finish restore process: %v", remoteError)
	}
	return nil
}

func finishAttempt(client *Client, closer closerFunc) (error, error) {
	var remoteError error
	defer closer()
	err := client.facade.FacadeCall("FinishRestore", nil, &remoteError)
	return err, remoteError
}

// finishRestore since Restore call will end up with a reset
// state server, finish restore will check that the the newly
// placed state server has the mark of restore complete.
// upstart should have restarted the api server so we reconnect.
func finishRestore(newClient ClientConnection) error {
	var err, remoteError error
	for a := restoreStrategy.Start(); a.Next(); {
		logger.Debugf("Attempting finishRestore")
		finishClient, finishClientCloser, err := newClient()
		if err != nil {
			return errors.Trace(err)
		}

		if err, remoteError = finishAttempt(finishClient, finishClientCloser); err == nil {
			return nil
		}
		if !params.IsCodeUpgradeInProgress(remoteError) {
			return errors.Annotatef(err, "cannot complete restore: %v", remoteError)
		}
	}
	return errors.Annotatef(err, "cannot complete restore: %v", remoteError)
}
