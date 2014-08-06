// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"time"

	"launchpad.net/tomb"
)

// periodicWorker implements the worker returned by NewPeriodicWorker.
type periodicWorker struct {
	tomb tomb.Tomb
}

// NewPeriodicWorker returns a worker that runs the given function continually
// sleeping for sleepDuration in between each call, until Kill() is called
// The stopCh argument will be closed when the worker is killed. The error returned
// by the doWork function will be returned by the worker's Wait function.
func NewPeriodicWorker(doWork func(stopCh <-chan struct{}) error, sleepDuration time.Duration) Worker {
	w := &periodicWorker{}
	timer := time.NewTimer(0)
	go func() {
		defer w.tomb.Done()
		for {
			if err := doWork(w.tomb.Dying()); err != nil {
				w.tomb.Kill(err)
				return
			}
			timer.Reset(sleepDuration)
			select {
			case <-timer.C:
			case <-w.tomb.Dying():
				w.tomb.Kill(tomb.ErrDying)
				return
			}
		}
	}()
	return w
}

// Kill implements Worker.Kill() and will close the channel given to the doWork
// function.
func (w *periodicWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.Wait(), and will return the error returned by
// the doWork function.
func (w *periodicWorker) Wait() error {
	return w.tomb.Wait()
}
