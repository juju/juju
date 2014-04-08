// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.singular")

var PingInterval = 10 * time.Second

type runner struct {
	pingErr         error
	pingerDied      chan struct{}
	startPingerOnce sync.Once
	isMaster        bool
	worker.Runner
	conn Conn
}

// Conn represents a connection to some resource.
type Conn interface {
	// IsMaster reports whether this connection is currently held by
	// the (singular) master of the resource.
	IsMaster() (bool, error)

	// Ping probes the resource and returns an error if the the
	// connection has failed. If the master changes, this method
	// must return an error.
	Ping() error
}

// New returns a Runner that can be used to start workers that will only
// run a single instance. The conn value is used to determine whether to
// run the workers or not.
//
// If conn.IsMaster returns true, any workers started will be started on the
// underlying runner.
//
// If conn.IsMaster returns false, any workers started will actually
// start do-nothing placeholder workers on the underlying runner
// that continually ping the connection until a ping fails and then exit
// with that error.
func New(underlying worker.Runner, conn Conn) (worker.Runner, error) {
	isMaster, err := conn.IsMaster()
	if err != nil {
		return nil, fmt.Errorf("cannot get master status: %v", err)
	}
	logger.Infof("runner created; isMaster %v", isMaster)
	return &runner{
		isMaster:   isMaster,
		Runner:     underlying,
		conn:       conn,
		pingerDied: make(chan struct{}),
	}, nil
}

// pinger periodically pings the connection to make sure that the
// master-status has not changed. When the ping fails, it sets r.pingErr
// to the error and closes r.pingerDied to signal the other workers to
// quit.
func (r *runner) pinger() {
	underlyingDead := make(chan struct{})
	go func() {
		r.Runner.Wait()
		close(underlyingDead)
	}()
	timer := time.NewTimer(0)
	for {
		if err := r.conn.Ping(); err != nil {
			// The ping has failed: cause all other workers
			// to exit with the ping error.
			logger.Infof("pinger has died: %v", err)
			r.pingErr = err
			close(r.pingerDied)
			return
		}
		timer.Reset(PingInterval)
		select {
		case <-timer.C:
		case <-underlyingDead:
			return
		}
	}
}

func (r *runner) StartWorker(id string, startFunc func() (worker.Worker, error)) error {
	if r.isMaster {
		// We are master; the started workers should
		// encounter an error as they do what they're supposed
		// to do - we can just start the worker in the
		// underlying runner.
		logger.Infof("starting %q", id)
		return r.Runner.StartWorker(id, startFunc)
	}
	logger.Infof("standby %q", id)
	// We're not master, so don't start the worker, but start a pinger so
	// that we know when the connection master changes.
	r.startPingerOnce.Do(func() {
		go r.pinger()
	})
	return r.Runner.StartWorker(id, func() (worker.Worker, error) {
		return worker.NewSimpleWorker(r.waitPinger), nil
	})
}

func (r *runner) waitPinger(stop <-chan struct{}) error {
	select {
	case <-stop:
		return nil
	case <-r.pingerDied:
		return r.pingErr
	}
}
