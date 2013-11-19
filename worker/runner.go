// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"errors"
	"time"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/log"
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

// Runner is implemented by instances capable of starting and stopping workers.
type Runner interface {
	Worker
	StartWorker(id string, startFunc func() (Worker, error)) error
	StopWorker(id string) error
}

// runner runs a set of workers, restarting them as necessary
// when they fail.
type runner struct {
	tomb          tomb.Tomb
	startc        chan startReq
	stopc         chan string
	donec         chan doneInfo
	startedc      chan startInfo
	isFatal       func(error) bool
	moreImportant func(err0, err1 error) bool
}

var _ Runner = (*runner)(nil)

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
// will be stopped and the runner itself will finish.  Of all the fatal errors
// returned by the stopped workers, only the most important one,
// determined by calling moreImportant, will be returned from
// Runner.Wait. Non-fatal errors will not be returned.
//
// The function isFatal(err) returns whether err is a fatal error.  The
// function moreImportant(err0, err1) returns whether err0 is considered
// more important than err1.
func NewRunner(isFatal func(error) bool, moreImportant func(err0, err1 error) bool) Runner {
	runner := &runner{
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
func (runner *runner) StartWorker(id string, startFunc func() (Worker, error)) error {
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
func (runner *runner) StopWorker(id string) error {
	select {
	case runner.stopc <- id:
		return nil
	case <-runner.tomb.Dead():
	}
	return ErrDead
}

func (runner *runner) Wait() error {
	return runner.tomb.Wait()
}

func (runner *runner) Kill() {
	log.Debugf("worker: killing runner %p", runner)
	runner.tomb.Kill(nil)
}

// Stop kills the given worker and waits for it to exit.
func Stop(worker Worker) error {
	worker.Kill()
	return worker.Wait()
}

type workerInfo struct {
	start        func() (Worker, error)
	worker       Worker
	restartDelay time.Duration
	stopping     bool
}

func (runner *runner) run() error {
	// workers holds the current set of workers.  All workers with a
	// running goroutine have an entry here.
	workers := make(map[string]*workerInfo)
	var finalError error

	// isDying holds whether the runner is currently dying.  When it
	// is dying (whether as a result of being killed or due to a
	// fatal error), all existing workers are killed, no new workers
	// will be started, and the loop will exit when all existing
	// workers have stopped.
	isDying := false
	tombDying := runner.tomb.Dying()
	for {
		if isDying && len(workers) == 0 {
			return finalError
		}
		select {
		case <-tombDying:
			log.Infof("worker: runner is dying")
			isDying = true
			killAll(workers)
			tombDying = nil
		case req := <-runner.startc:
			if isDying {
				log.Infof("worker: ignoring start request for %q when dying", req.id)
				break
			}
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
			// The worker previously existed and is
			// currently being stopped.  When it eventually
			// does stop, we'll restart it immediately with
			// the new start function.
			info.start = req.start
			info.restartDelay = 0
		case id := <-runner.stopc:
			if info := workers[id]; info != nil {
				killWorker(id, info)
			}
		case info := <-runner.startedc:
			workerInfo := workers[info.id]
			workerInfo.worker = info.worker
			if isDying {
				killWorker(info.id, workerInfo)
			}
		case info := <-runner.donec:
			workerInfo := workers[info.id]
			if !workerInfo.stopping && info.err == nil {
				info.err = errors.New("unexpected quit")
			}
			if info.err != nil {
				if runner.isFatal(info.err) {
					log.Errorf("worker: fatal %q: %v", info.id, info.err)
					if finalError == nil || runner.moreImportant(info.err, finalError) {
						finalError = info.err
					}
					delete(workers, info.id)
					if !isDying {
						isDying = true
						killAll(workers)
					}
					break
				} else {
					log.Errorf("worker: exited %q: %v", info.id, info.err)
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
}

func killAll(workers map[string]*workerInfo) {
	for id, info := range workers {
		killWorker(id, info)
	}
}

func killWorker(id string, info *workerInfo) {
	if info.worker != nil {
		log.Debugf("worker: killing %q", id)
		info.worker.Kill()
		info.worker = nil
	}
	info.stopping = true
	info.start = nil
}

// runWorker starts the given worker after waiting for the given delay.
func (runner *runner) runWorker(delay time.Duration, id string, start func() (Worker, error)) {
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
