// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"io"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"
)

type proxyConfig[T io.Closer] struct {
	// createRemote takes an SSH connection to a remote machine and
	//  returns a protocol specific object for proxying the user's request.
	createRemote func(context.Context, *gossh.Client) (T, error)
	// run performs the handler-specific proxy operation.
	run func(T) error
	// onError handles errors produced during the proxy lifecycle.
	onError func(error)
}

// handleProxy is a generic helper for implementing SSH handlers that proxy to a remote machine.
// It performs the following steps:
// 1. Establishes an SSH connection to the target machine.
// 2. Creates a protocol-specific object for proxying the user's request.
// 3. Runs the handler-specific proxy operation.
// 4. Handles any errors that occur during the proxy lifecycle.
func handleProxy[T io.Closer](h *Handlers, ctx context.Context, cfg proxyConfig[T]) {
	client, err := h.connector.Connect(ctx, h.destination)
	if err != nil {
		cfg.onError(errors.Annotate(err, "failed to connect to machine"))
		return
	}
	defer client.Close()

	remote, err := cfg.createRemote(ctx, client)
	if err != nil {
		cfg.onError(errors.Annotate(err, "failed to create remote proxy"))
		return
	}
	defer remote.Close()

	// Tear down both connections when the context is done.
	// This context is the user's SSH connection context, so when
	// the user disconnects, we will close the connection to the machine.
	stop := context.AfterFunc(ctx, func() {
		_ = client.Close()
		_ = remote.Close()
	})
	defer stop()

	if err := cfg.run(remote); err != nil {
		cfg.onError(err)
	}
}

func (h *Handlers) handleError(session ssh.Session, err error) {
	h.logger.Errorf(session.Context(), "machine proxy failure: %v", err)
	_, _ = session.Stderr().Write([]byte(err.Error() + "\n"))

	var exitErr *gossh.ExitError
	if errors.As(err, &exitErr) {
		_ = session.Exit(exitErr.ExitStatus())
		return
	}
	_ = session.Exit(1)
}

// proxyStreams proxies data between two streams and attempts
// to close the write side of each stream when there is no more
// data to read from the other stream.
// See https://github.com/golang/go/issues/35892 for a scenario
// where this is relevant.
func proxyStreams(left, right io.ReadWriteCloser) {
	var wg sync.WaitGroup
	wg.Go(func() {
		_, _ = io.Copy(left, right)
		closeWrite(left)
	})
	wg.Go(func() {
		_, _ = io.Copy(right, left)
		closeWrite(right)
	})
	wg.Wait()
}

func closeWrite(closer io.ReadWriteCloser) {
	if halfCloser, ok := closer.(interface{ CloseWrite() error }); ok {
		_ = halfCloser.CloseWrite()
		return
	}
	_ = closer.Close()
}
