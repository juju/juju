// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	"github.com/juju/juju/state"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"
)

// sshServerWorker is a worker that runs an embedded SSH server.
// See the server implementation for more details on how it works.
type sshServerWorker struct {
	tomb tomb.Tomb

	srv *ssh.Server
}

// NewSSHServerWorker returns a new worker that runs an embedded SSH server.
func NewSSHServerWorker(statePool *state.StatePool, jumpHostKey, terminatingHostKey string) (worker.Worker, error) {
	srv, err := NewSSHServer(statePool, jumpHostKey, terminatingHostKey)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &sshServerWorker{
		srv: srv,
	}

	w.tomb.Go(w.srv.ListenAndServe)
	return w, nil
}

// Kill implements worker.Worker. It kills the worker by calling
// the servers (graceful) Shutdown method and then killing the tomb.
func (ssw *sshServerWorker) Kill() {
	ssw.srv.Shutdown(context.Background())
	ssw.tomb.Kill(nil)
}

// Wait implements worker.Worker. It waits on the tomb wrapping
// the servers ListenAndServe method.
func (ssw *sshServerWorker) Wait() error {
	return ssw.tomb.Wait()
}
