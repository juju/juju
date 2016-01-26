// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spool

import (
	"net"
	"time"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/worker"
)

const (
	// DefaultTimeout specifies the default socket read and write timeout.
	DefaultTimeout = 3 * time.Second
)

// ConnectionHandler defines the method needed to handle socket connections.
type ConnectionHandler interface {
	Handle(net.Conn) error
}

type socketListener struct {
	listener net.Listener
	t        tomb.Tomb

	handler ConnectionHandler
}

// NewSocketListener returns a new socket listener struct.
func NewSocketListener(socketPath string, handler ConnectionHandler) (*socketListener, error) {
	listener, err := sockets.Listen(socketPath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sListener := &socketListener{listener: listener, handler: handler}
	go func() {
		defer sListener.t.Done()
		sListener.t.Kill(sListener.loop())
	}()
	return sListener, nil
}

// Stop closes the listener and releases all resources
// used by the socketListener.
func (l *socketListener) Stop() {
	l.t.Kill(nil)
	err := l.listener.Close()
	if err != nil {
		logger.Errorf("failed to close the collect-metrics listener: %v", err)
	}
	err = l.t.Wait()
	if err != nil {
		logger.Errorf("failed waiting for all goroutines to finish: %v", err)
	}
}

func (l *socketListener) loop() (_err error) {
	defer func() {
		select {
		case <-l.t.Dying():
			_err = nil
		default:
		}
	}()
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			return errors.Trace(err)
		}
		go func() error {
			err := l.handler.Handle(conn)
			if err != nil {
				// log the error and continue
				logger.Errorf("request handling failed: %v", err)
			}
			return nil
		}()
	}
	return
}

// NewPeriodicWorker returns a periodic worker, that will call a stop function
// when it is killed.
func NewPeriodicWorker(do worker.PeriodicWorkerCall, period time.Duration, newTimer func(time.Duration) worker.PeriodicTimer, stop func()) worker.Worker {
	return &periodicWorker{
		Worker: worker.NewPeriodicWorker(do, period, newTimer),
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
