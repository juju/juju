// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"fmt"

	"github.com/juju/loggo"
	"launchpad.net/tomb"

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
		a.tomb.Kill(u.loop())
	}()
	return a
}

func (a *addresserWorker) Kill() {
	a.tomb.Kill(nil)
}

func (a *addresserWorker) Wait() error {
	return u.tomb.Wait()
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

	return nil
}

func watchAddressesLoop(addresser addresserWorker, w state.StringsWatcher) (err error) {
	defer func() {
		if stopErr := w.Stop(); stopErr != nil {
			if err == nil {
				err = fmt.Errorf("error stopping watcher: %v", stopErr)
			} else {
				logger.Warningf("ignoring error when stopping watcher: %v", stopErr)
			}
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
