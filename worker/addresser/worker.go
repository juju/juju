// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.addresser")

type releaser interface {
	// ReleaseAddress has the same signature as the same method in the
	// NetworkingEnviron
	ReleaseAddress(instId instance.Id, subnetId network.Id, addr network.Address) error
}

type addresserWorker struct {
	st       *state.State
	tomb     tomb.Tomb
	releaser releaser

	observer *worker.EnvironObserver
}

// NewWorker returns a worker that keeps track of
// IP address lifecycles, removing Dead addresses.
func NewWorker(st *state.State) (worker.Worker, error) {
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	environ, err := environs.New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	netEnviron, ok := environs.SupportsNetworking(environ)
	if !ok {
		return nil, errors.New("environment does not support networking")
	}
	a := NewWorkerWithReleaser(st, netEnviron)
	return a, nil
}

func NewWorkerWithReleaser(st *state.State, releaser releaser) worker.Worker {
	a := &addresserWorker{
		st:       st,
		releaser: releaser,
	}
	// wait for environment
	go func() {
		defer a.tomb.Done()
		a.tomb.Kill(a.loop())
	}()
	return a
}

func (a *addresserWorker) Kill() {
	a.tomb.Kill(nil)
}

func (a *addresserWorker) Wait() error {
	return a.tomb.Wait()
}

func (a *addresserWorker) loop() (err error) {
	a.observer, err = worker.NewEnvironObserver(a.st)
	if err != nil {
		return err
	}
	logger.Infof("addresser received inital environment configuration")
	defer func() {
		obsErr := worker.Stop(a.observer)
		if err == nil {
			err = obsErr
		}
	}()
	return watchAddressesLoop(a, a.st.WatchIPAddresses())
}

func (a *addresserWorker) dying() <-chan struct{} {
	return a.tomb.Dying()
}

func (a *addresserWorker) killAll(err error) {
	a.tomb.Kill(err)
}

func (a *addresserWorker) checkAddresses(ids []string) error {
	for _, id := range ids {
		addr, err := a.st.IPAddress(id)
		if err != nil {
			return err
		}
		if addr.Life() != state.Dead {
			continue
		}
		err = addr.Remove()
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *addresserWorker) removeIPAddress(addr *state.IPAddress) error {
	// XXX addr.MachineId is wrong here, it needs to be instance ID which we
	// get from state
	err := a.releaser.ReleaseAddress(instance.Id(addr.MachineId()), network.Id(addr.SubnetId()), addr.Address())
	if err != nil {
		// Don't remove the address from state so we
		// can retry releasing the address later.
		logger.Warningf("failed to release address %v: %v", addr.Value, err)
		return errors.Trace(err)
	}

	err = addr.Remove()
	if err != nil {
		logger.Warningf("failed to remove address %v: %v", addr.Value, err)
		return errors.Trace(err)
	}
	return nil
}

func watchAddressesLoop(addresser *addresserWorker, w state.StringsWatcher) (err error) {
	defer func() {
		if stopErr := w.Stop(); stopErr != nil {
			if err == nil {
				err = fmt.Errorf("error stopping watcher: %v", stopErr)
			} else {
				logger.Warningf("ignoring error when stopping watcher: %v", stopErr)
			}
		}
	}()

	dead, err := addresser.st.DeadIPAddresses()
	if err != nil {
		return errors.Trace(err)
	}
	deadQueue := make(chan *state.IPAddress, len(dead))
	for _, deadAddr := range dead {
		deadQueue <- deadAddr
	}
	go func() {
		select {
		case addr := <-deadQueue:
			err := addresser.removeIPAddress(addr)
			if err != nil {
				logger.Warningf("error releasing dead IP address %q: %v", addr, err)
			}
		case <-addresser.dying():
			return
		default:
			return
		}
	}()

	for {
		select {
		case ids, ok := <-w.Changes():
			if !ok {
				return watcher.EnsureErr(w)
			}
			if err := addresser.checkAddresses(ids); err != nil {
				return err
			}
		case <-addresser.dying():
			return nil
		}
	}
}
