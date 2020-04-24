// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"
	"math/rand"
	"time"

	"github.com/juju/worker/v2"
	"gopkg.in/tomb.v2"
)

// ErrKilled can be returned by the PeriodicWorkerCall to signify that
// the function has returned as a result of a Stop() or Kill() signal
// and that the function was able to stop cleanly
var ErrKilled = errors.New("worker killed")

// PeriodicWorkerOption is an optional parameter of the NewPeriodicWorker function and can be
// used to set optional parameters of the new periodic worker.
type PeriodicWorkerOption func(w *periodicWorker)

// Jitter will introduce a jitter in the worker's period by the specified amount (as percents - i.e. between 0 and 1).
func Jitter(amount float64) PeriodicWorkerOption {
	return func(w *periodicWorker) {
		w.jitter = amount
	}
}

// periodicWorker implements the worker returned by NewPeriodicWorker.
type periodicWorker struct {
	jitter   float64
	tomb     tomb.Tomb
	newTimer NewTimerFunc
}

// PeriodicWorkerCall represents the callable to be passed
// to the periodic worker to be run every elapsed period.
type PeriodicWorkerCall func(stop <-chan struct{}) error

// PeriodicTimer is an interface for the timer that periodicworker
// will use to handle the calls.
type PeriodicTimer interface {
	// Reset changes the timer to expire after duration d.
	// It returns true if the timer had been active, false
	// if the timer had expired or been stopped.
	Reset(time.Duration) bool
	// CountDown returns the channel used to signal expiration of
	// the timer duration. The channel is called C in the base
	// implementation of timer but the name is confusing.
	CountDown() <-chan time.Time
}

// NewTimerFunc is a constructor used to obtain the instance
// of PeriodicTimer periodicWorker uses on its loop.
// TODO(fwereade): 2016-03-17 lp:1558657
type NewTimerFunc func(time.Duration) PeriodicTimer

// Timer implements PeriodicTimer.
type Timer struct {
	timer *time.Timer
}

// Reset implements PeriodicTimer.
func (t *Timer) Reset(d time.Duration) bool {
	return t.timer.Reset(d)
}

// CountDown implements PeriodicTimer.
func (t *Timer) CountDown() <-chan time.Time {
	return t.timer.C
}

// NewTimer is the default implementation of NewTimerFunc.
func NewTimer(d time.Duration) PeriodicTimer {
	return &Timer{time.NewTimer(d)}
}

// NewPeriodicWorker returns a worker that runs the given function continually
// sleeping for sleepDuration in between each call, until Kill() is called
// The stopCh argument will be closed when the worker is killed. The error returned
// by the doWork function will be returned by the worker's Wait function.
func NewPeriodicWorker(call PeriodicWorkerCall, period time.Duration, timerFunc NewTimerFunc, options ...PeriodicWorkerOption) worker.Worker {
	w := &periodicWorker{newTimer: timerFunc}
	for _, option := range options {
		option(w)
	}
	w.tomb.Go(func() error {
		return w.run(call, period)
	})
	return w
}

func (w *periodicWorker) run(call PeriodicWorkerCall, period time.Duration) error {
	timer := w.newTimer(0)
	stop := w.tomb.Dying()
	for {
		select {
		case <-stop:
			return tomb.ErrDying
		case <-timer.CountDown():
			if err := call(stop); err != nil {
				if err == ErrKilled {
					return tomb.ErrDying
				}
				return err
			}
		}
		timer.Reset(nextPeriod(period, w.jitter))
	}
}

var nextPeriod = func(period time.Duration, jitter float64) time.Duration {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	p := period
	if jitter != 0 {
		lower := (1.0 - jitter) * float64(period)
		window := (2.0 * jitter) * float64(period)
		offset := float64(r.Int63n(int64(window)))
		p = time.Duration(lower + offset)
	}
	return p
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
