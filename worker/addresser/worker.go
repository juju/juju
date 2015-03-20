// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.addresser")

type addresserWorker struct {
	st   *state.State
	tomb tomb.Tomb

	observer *worker.EnvironObserver
}

// NewWorker returns a worker that keeps track of
// IP address lifecycles, removing Dead addresses.
func NewWorker(st *state.State) worker.Worker {
	a := &addresserWorker{
		st: st,
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
	err := a.environ.ReleaseAddress(ciid, network.Id(addr.SubnetId()), addr.Address())
	if err != nil {
		// Don't remove the address from state so we
		// can retry releasing the address later.
		logger.Warningf("failed to release address %v for container %q: %v", addr.Value, tag, err)
		return errors.Trace(err)
	}

	err = addr.Remove()
	if err != nil {
		logger.Warningf("failed to remove address %v for container %q: %v", addr.Value, tag, err)
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
			err := addr.Remove()
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
