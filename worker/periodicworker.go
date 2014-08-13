// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"
	"time"

	"launchpad.net/tomb"
)

// ErrKilled can be returned by the PeriodicWorkerCall to signify that
// the function has returned as a result of a Stop() or Kill() signal
// and that the function was able to stop cleanly
var ErrKilled = errors.New("worker killed")

// periodicWorker implements the worker returned by NewPeriodicWorker.
type periodicWorker struct {
	tomb tomb.Tomb
}

type PeriodicWorkerCall func(stop <-chan struct{}) error

// NewPeriodicWorker returns a worker that runs the given function continually
// sleeping for sleepDuration in between each call, until Kill() is called
// The stopCh argument will be closed when the worker is killed. The error returned
// by the doWork function will be returned by the worker's Wait function.
func NewPeriodicWorker(call PeriodicWorkerCall, period time.Duration) Worker {
	w := &periodicWorker{}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.run(call, period))
	}()
	return w
}

func (w *periodicWorker) run(call PeriodicWorkerCall, period time.Duration) error {
	timer := time.NewTimer(0)
	stop := w.tomb.Dying()
	for {
		select {
		case <-stop:
			return tomb.ErrDying
		case <-timer.C:
			if err := call(stop); err != nil {
				if err == ErrKilled {
					return tomb.ErrDying
				}
				return err
			}
		}
		timer.Reset(period)
	}
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
