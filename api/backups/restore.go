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

var (
	// TODO(perrito666): this Key needs to be determined in a dinamic way since machine 0
	// might not always be there in HA contexts.
	// RestoreMachineKey holds the value for the machine where we restore, this should be
	// obtained from the running environment.
	RestoreMachineKey = "0"
	// restoreStrategy is the attempt strategy for api server calls re-attempts in case
	// the server is upgrading.
	restoreStrategy = utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Min:   10,
	}
)

// ClientConnection type represents a function capable of spawning a new Client connection
// it is used to pass around connection factories when necessary.
type ClientConnection func() (*Client, func() error, error)

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
		defer clientCloser()
		if err = client.facade.FacadeCall("PrepareRestore", nil, &remoteError); err == nil {
			break
		}
		if !params.IsCodeUpgradeInProgress(remoteError) {
			return errors.Annotatef(err, "could not start prepare restore mode, server returned: %v", remoteError)
		}
	}
	return errors.Annotatef(err, "could not start restore process: %v", remoteError)
}

// RestoreReader restores the contents of backupFile as backup.
func (c *Client) RestoreReader(r io.Reader, newClient ClientConnection) error {
	if err := prepareRestore(newClient); err != nil {
		errors.Trace(err)
	}
	logger.Debugf("Server is now in 'about to restore' mode, proceeding to upload the backup file")

	// Upload
	backupId, err := c.Upload(r)
	if err != nil {
		//TODO(perrito666): this is a recoverable error, we should undo Prepare
		return errors.Annotatef(err, "cannot upload backup file")
	}
	return c.restore(backupId, newClient)
}

// Restore performs restore using a backup id corresponding to a backup stored in the server.
func (c *Client) Restore(backupId string, newClient ClientConnection) error {
	if err := prepareRestore(newClient); err != nil {
		errors.Trace(err)
	}
	logger.Debugf("Server in 'about to restore' mode")
	return c.restore(backupId, newClient)
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
		Machine:  RestoreMachineKey,
	}

	for a := restoreStrategy.Start(); a.Next(); {
		logger.Debugf("Attempting Restore of %q", backupId)
		restoreClient, restoreClientCloser, err := newClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer restoreClientCloser()

		err = restoreClient.facade.FacadeCall("Restore", restoreArgs, &remoteError)

		// This signals that Restore almost certainly finished and
		// triggered Exit.
		if err == rpc.ErrShutdown && remoteError == nil {
			break
		}
		//TODO(perrito666): There are some of the possible outcomes that might
		// deserve disable PrepareRestore mode
		if err != nil && !params.IsCodeUpgradeInProgress(remoteError) {
			return errors.Annotatef(err, "cannot perform restore: %v", remoteError)
		}
	}
	if err != rpc.ErrShutdown {
		return errors.Annotatef(err, "cannot perform restore: %v", remoteError)
	}

	// FinishRestore since Restore call will end up with a reset
	// state server, finish restore will check that the the newly
	// placed state server has the mark of restore complete.
	// upstart should have restarted the api server so we reconnect.
	for a := restoreStrategy.Start(); a.Next(); {
		logger.Debugf("Attempting FinishRestore")
		finishClient, finishClientCloser, err := newClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer finishClientCloser()

		err = finishClient.facade.FacadeCall("FinishRestore", nil, &remoteError)
		if err == nil {
			break
		}
		if !params.IsCodeUpgradeInProgress(remoteError) {
			return errors.Annotatef(err, "cannot complete restore: %v", remoteError)
		}
	}
	if err != nil {
		return errors.Annotatef(err, "could not finish restore process: %v", remoteError)
	}
	return nil
}
