// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"
	"launchpad.net/juju-core/log"
	"launchpad.net/tomb"
	"time"
	"local/runtime/debug"
)

// RestartDelay holds the length of time that a worker
// will wait between exiting and restarting.
var RestartDelay = 3 * time.Second

// Worker is implemented by a running worker.
type Worker interface {
	// Kill asks the worker to stop without necessarily
	// waiting for it to do so.
	Kill()
	// Wait waits for the worker to exit and returns any
	// error encountered when it was running.
	Wait() error
}

// Runner runs a set of workers, restarting them as necessary
// when they fail.
type Runner struct {
	tomb          tomb.Tomb
	startc        chan startReq
	stopc         chan string
	donec         chan doneInfo
	startedc      chan startInfo
	isFatal       func(error) bool
	moreImportant func(err0, err1 error) bool
}

type startReq struct {
	id    string
	start func() (Worker, error)
}

type startInfo struct {
	id     string
	worker Worker
}

type doneInfo struct {
	id  string
	err error
}

// NewRunner creates a new Runner.  When a worker finishes, if its error
// is deemed fatal (determined by calling isFatal), all the other workers
// will be stopped and the runner itself will finish.  Of all the errors
// returned by the stopped workers, only the most important one
// (determined by calling moreImportant) will be returned from
// Runner.Wait.
//
// The function isFatal(err) returns whether err is a fatal error.  The
// function moreImportant(err0, err1) returns whether err0 is considered
// more important than err1..
func NewRunner(isFatal func(error) bool, moreImportant func(err0, err1 error) bool) *Runner {
	runner := &Runner{
		startc:        make(chan startReq),
		stopc:         make(chan string),
		donec:         make(chan doneInfo),
		startedc:      make(chan startInfo),
		isFatal:       isFatal,
		moreImportant: moreImportant,
	}
	go func() {
		defer runner.tomb.Done()
		runner.tomb.Kill(runner.run())
	}()
	return runner
}

var ErrDead = errors.New("worker runner is not running")

// StartWorker starts a worker running associated with the given id.
// The startFunc function will be called to create the worker;
// when the worker exits, it will be restarted as long as it
// does not return a fatal error.
//
// If there is already a worker with the given id, nothing will be done.
//
// StartWorker returns ErrDead if the runner is not running.
func (runner *Runner) StartWorker(id string, startFunc func() (Worker, error)) error {
	select {
	case runner.startc <- startReq{id, startFunc}:
		return nil
	case <-runner.tomb.Dead():
	}
	return ErrDead
}

// StopWorker stops the worker associated with the given id.
// It does nothing if there is no such worker.
//
// StopWorker returns ErrDead if the runner is not running.
func (runner *Runner) StopWorker(id string) error {
	select {
	case runner.stopc <- id:
		return nil
	case <-runner.tomb.Dead():
	}
	return ErrDead
}

func (runner *Runner) Wait() error {
	return runner.tomb.Wait()
}

func (runner *Runner) Kill() {
	log.Debugf("worker: killing runner %p %s", runner, debug.Callers(0, 20))
	runner.tomb.Kill(nil)
}

// Stop kills the given worker and waits for it to exit.
func Stop(worker Worker) error {
	worker.Kill()
	return worker.Wait()
}

func (runner *Runner) run() error {
	type workerInfo struct {
		start        func() (Worker, error)
		worker       Worker
		restartDelay time.Duration
		stopping     bool
	}
	// workers holds the current set of workers.  All workers with a
	// running goroutine have an entry here.
	workers := make(map[string]*workerInfo)
	var finalError error
loop:
	for {
		select {
		case <-runner.tomb.Dying():
			log.Infof("runner %p dying", runner)
			break loop
		case req := <-runner.startc:
			info := workers[req.id]
			if info == nil {
				workers[req.id] = &workerInfo{
					start:        req.start,
					restartDelay: RestartDelay,
				}
				go runner.runWorker(0, req.id, req.start)
				break
			}
			if !info.stopping {
				// The worker is already running, so leave it alone
				break
			}
			// The worker previously existed and is currently
			// being stopped.  When it eventually does stop,
			// we'll restart it immediately with the new
			// start function.
			info.start = req.start
			info.restartDelay = 0
		case id := <-runner.stopc:
			info := workers[id]
			if info == nil {
				// The worker doesn't exist so nothing to do.
				break
			}
			if info.worker != nil {
				log.Debugf("worker: killing %q", id)
				info.worker.Kill()
				info.worker = nil
			}
			info.stopping = true
			info.start = nil
		case info := <-runner.startedc:
			workers[info.id].worker = info.worker
		case info := <-runner.donec:
			workerInfo := workers[info.id]
			if !workerInfo.stopping && info.err == nil {
				info.err = errors.New("unexpected quit")
			}
			if info.err != nil {
				log.Errorf("worker: exited %q: %v", info.id, info.err)
				if runner.isFatal(info.err) {
					finalError = info.err
					delete(workers, info.id)
					break loop
				}
			}
			if workerInfo.start == nil {
				// The worker has been deliberately stopped;
				// we can now remove it from the list of workers.
				delete(workers, info.id)
				break
			}
			go runner.runWorker(workerInfo.restartDelay, info.id, workerInfo.start)
			workerInfo.restartDelay = RestartDelay
		}
	}
	for id, info := range workers {
		if info.worker != nil {
			log.Debugf("worker: killing %q", id)
			info.worker.Kill()
			info.worker = nil
		}
	}
	for len(workers) > 0 {
		info := <-runner.donec
		if runner.moreImportant(info.err, finalError) {
			finalError = info.err
		}
		if true || info.err != nil {
			log.Errorf("worker: %q exited: %v", info.id, info.err)
		}
		delete(workers, info.id)
	}
	return finalError
}

// runWorker starts the given worker after waiting for the given delay.
func (runner *Runner) runWorker(delay time.Duration, id string, start func() (Worker, error)) {
	if delay > 0 {
		log.Infof("worker: restarting %q in %v", id, delay)
		select {
		case <-runner.tomb.Dying():
			runner.donec <- doneInfo{id, nil}
			return
		case <-time.After(delay):
		}
	}
	log.Infof("worker: start %q", id)
	worker, err := start()
	if err == nil {
		runner.startedc <- startInfo{id, worker}
		err = worker.Wait()
	}
	runner.donec <- doneInfo{id, err}
}
