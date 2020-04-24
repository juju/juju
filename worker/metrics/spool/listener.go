// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spool

import (
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/juju/sockets"
	jworker "github.com/juju/juju/worker"
)

const (
	// DefaultTimeout specifies the default socket read and write timeout.
	DefaultTimeout = 3 * time.Second
)

// ConnectionHandler defines the method needed to handle socket connections.
type ConnectionHandler interface {
	Handle(net.Conn, <-chan struct{}) error
}

type socketListener struct {
	listener net.Listener
	t        tomb.Tomb

	handler ConnectionHandler
}

// NewSocketListener returns a new socket listener struct.
func NewSocketListener(socketPath string, handler ConnectionHandler) (*socketListener, error) {
	listener, err := sockets.Listen(sockets.Socket{Network: "unix", Address: socketPath})
	if err != nil {
		return nil, errors.Trace(err)
	}
	sListener := &socketListener{listener: listener, handler: handler}
	sListener.t.Go(sListener.loop)
	return sListener, nil
}

// Stop closes the listener and releases all resources
// used by the socketListener.
func (l *socketListener) Stop() error {
	l.t.Kill(nil)
	err := l.listener.Close()
	if err != nil {
		logger.Errorf("failed to close the collect-metrics listener: %v", err)
	}
	return l.t.Wait()
}

func (l *socketListener) loop() error {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			return errors.Trace(err)
		}
		go func() {
			err := l.handler.Handle(conn, l.t.Dying())
			if err != nil {
				// log the error and continue
				logger.Errorf("request handling failed: %v", err)
			}
		}()
	}
}

// NewPeriodicWorker returns a periodic worker, that will call a stop function
// when it is killed.
func NewPeriodicWorker(do jworker.PeriodicWorkerCall, period time.Duration, newTimer func(time.Duration) jworker.PeriodicTimer, stop func()) worker.Worker {
	return &periodicWorker{
		Worker: jworker.NewPeriodicWorker(do, period, newTimer, jworker.Jitter(0.2)),
		stop:   stop,
	}
}

type periodicWorker struct {
	worker.Worker
	stop func()
}

// Kill implements the worker.Worker interface.
func (w *periodicWorker) Kill() {
	w.stop()
	w.Worker.Kill()
}
