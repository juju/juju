// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"net"

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
}

type sshTunnelerWorker struct {
	tomb          tomb.Tomb
	tunnelTracker *sshtunneler.TunnelTracker
}

type sshDialer struct{}

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

// NewWorker returns a worker that provides a JWTParser.
func NewWorker(client FacadeClient, tunnelerSecret *sshtunneler.TunnelSecret) (worker.Worker, error) {
	if tunnelerSecret == nil {
		return nil, errors.New("tunneler secret is required")
	}

	args := sshtunneler.TunnelTrackerArgs{
		State:          client,
		ControllerInfo: client,
		Dialer:         sshDialer{},
		TunnelSecret:   *tunnelerSecret,
	}
	tracker, err := sshtunneler.NewTunnelTracker(args)
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
