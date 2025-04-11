// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"net"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/sshtunneler"
	"github.com/juju/juju/state"
)

// FacadeClient represents the SSH tunneler's facade client.
type FacadeClient interface {
	InsertSSHConnRequest(arg state.SSHConnRequestArg) error
	Addresses() (network.SpaceAddresses, error)
	MachineHostKeys(modelUUID, machineID string) ([]string, error)
}

type sshTunnelerWorker struct {
	tomb          tomb.Tomb
	tunnelTracker *sshtunneler.Tracker
}

type sshDialer struct{}

// Dial establishes an SSH connection over the provided connection.
// It uses the provided username, private key, and host key callback for authentication.
func (d sshDialer) Dial(conn net.Conn, username string, privateKey gossh.Signer, hostKeyCallback gossh.HostKeyCallback) (*gossh.Client, error) {
	sshConn, newChan, reqs, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
		HostKeyCallback: hostKeyCallback,
		User:            username,
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(privateKey),
		},
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to establish SSH connection")
	}
	return gossh.NewClient(sshConn, newChan, reqs), nil
}

// NewWorker returns a worker that provides an SSH tunnel tracker.
func NewWorker(client FacadeClient, clock clock.Clock) (worker.Worker, error) {
	args := sshtunneler.TrackerArgs{
		State:          client,
		ControllerInfo: client,
		Dialer:         sshDialer{},
		Clock:          clock,
	}
	tracker, err := sshtunneler.NewTracker(args)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create tunnel tracker")
	}

	w := &sshTunnelerWorker{tunnelTracker: tracker}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *sshTunnelerWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *sshTunnelerWorker) Wait() error {
	return w.tomb.Wait()
}
