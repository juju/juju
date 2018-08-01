// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/tomb.v2"
)

// DefaultRestartDelay holds the default length of time that a worker
// will wait between exiting and being restarted by a Runner.
const DefaultRestartDelay = 3 * time.Second

var (
	ErrNotFound = errors.New("worker not found")
	ErrStopped  = errors.New("aborted waiting for worker")
	ErrDead     = errors.New("worker runner is not running")
)

// Runner runs a set of workers, restarting them as necessary
// when they fail.
type Runner struct {
	tomb     tomb.Tomb
	startc   chan startReq
	stopc    chan string
	donec    chan doneInfo
	startedc chan startInfo

	params RunnerParams

	// isDying is maintained by the run goroutine.
	// When it is dying (whether as a result of being killed or due to a
	// fatal error), all existing workers are killed, no new workers
	// will be started, and the loop will exit when all existing
	// workers have stopped.
	isDying bool

	// finalError is maintained by the run goroutine.
	// finalError holds the error that will be returned
	// when the runner finally exits.
	finalError error

	// mu guards the fields below it. Note that the
	// run goroutine only locks the mutex when
	// it changes workers, not when it reads it. It can do this
	// because it's the only goroutine that changes it.
	mu sync.Mutex

	// workersChangedCond is notified whenever the
	// current workers state changes.
	workersChangedCond sync.Cond

	// workers holds the current set of workers.
	workers map[string]*workerInfo
}

// workerInfo holds information on one worker id.
type workerInfo struct {
	// worker holds the current Worker instance. This field is
	// guarded by the Runner.mu mutex so it can be inspected
	// by Runner.Worker calls.
	worker Worker

	// The following fields are maintained by the
	// run goroutine.

	// start holds the function to create the worker.
	// If this is nil, the worker has been stopped
	// and will be removed when its goroutine exits.
	start func() (Worker, error)

	// restartDelay holds the length of time that runWorker
	// will wait before calling the start function.
	restartDelay time.Duration

	// stopping holds whether the worker is currently
	// being killed. The runWorker goroutine will
	// still exist while this is true.
	stopping bool
}

type startReq struct {
	id    string
	start func() (Worker, error)
	reply chan struct{}
}

type startInfo struct {
	id     string
	worker Worker
}

type doneInfo struct {
	id  string
	err error
}

// RunnerParams holds the parameters for a NewRunner call.
type RunnerParams struct {
	// IsFatal is called when a worker exits. If it returns
	// true, all the other workers
	// will be stopped and the runner itself will finish.
	//
	// If IsFatal is nil, all errors will be treated as fatal.
	IsFatal func(error) bool

	// When the runner exits because one or more
	// workers have returned a fatal error, only the most important one,
	// will be returned. MoreImportant should report whether
	// err0 is more important than err1.
	//
	// If MoreImportant is nil, the first error reported will be
	// returned.
	MoreImportant func(err0, err1 error) bool

	// RestartDelay holds the length of time the runner will
	// wait after a worker has exited with a non-fatal error
	// before it is restarted.
	// If this is zero, DefaultRestartDelay will be used.
	RestartDelay time.Duration

	// Clock is used for timekeeping. If it's nil, clock.WallClock
	// will be used.
	Clock clock.Clock
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
func NewRunner(p RunnerParams) *Runner {
	if p.IsFatal == nil {
		p.IsFatal = func(error) bool {
			return true
		}
	}
	if p.MoreImportant == nil {
		p.MoreImportant = func(err0, err1 error) bool {
			return true
		}
	}
	if p.RestartDelay == 0 {
		p.RestartDelay = DefaultRestartDelay
	}
	if p.Clock == nil {
		p.Clock = clock.WallClock
	}

	runner := &Runner{
		startc:   make(chan startReq),
		stopc:    make(chan string),
		donec:    make(chan doneInfo),
		startedc: make(chan startInfo),
		params:   p,
		workers:  make(map[string]*workerInfo),
	}
	runner.workersChangedCond.L = &runner.mu
	runner.tomb.Go(runner.run)
	return runner
}

// StartWorker starts a worker running associated with the given id.
// The startFunc function will be called to create the worker;
// when the worker exits, it will be restarted as long as it
// does not return a fatal error.
//
// If there is already a worker with the given id, nothing will be done.
//
// StartWorker returns ErrDead if the runner is not running.
func (runner *Runner) StartWorker(id string, startFunc func() (Worker, error)) error {
	// Note: we need the reply channel so that when StartWorker
	// returns, we're guaranteed that the worker is installed
	// when we return, so Worker will see it if called
	// immediately afterwards.
	reply := make(chan struct{})
	select {
	case runner.startc <- startReq{id, startFunc, reply}:
		// We're certain to get a reply because the startc channel is synchronous
		// so if we succeed in sending on it, we know that the run goroutine has entered
		// the startc arm of the select, and that calls startWorker (which never blocks)
		// and then immedaitely closes the reply channel.
		<-reply
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

// Wait implements Worker.Wait
func (runner *Runner) Wait() error {
	return runner.tomb.Wait()
}

// Kill implements Worker.Kill
func (runner *Runner) Kill() {
	logger.Debugf("killing runner %p", runner)
	runner.tomb.Kill(nil)
}

// Worker returns the current worker for the given id.
// If a worker has been started with the given id but is
// not currently available, it will wait until it is available,
// stopping waiting if it receives a value on the stop channel.
//
// If there is no worker started with the given id, Worker
// will return ErrNotFound. If it was stopped while
// waiting, Worker will return ErrStopped. If the runner
// has been killed while waiting, Worker will return ErrDead.
func (runner *Runner) Worker(id string, stop <-chan struct{}) (Worker, error) {
	runner.mu.Lock()
	// getWorker returns the current worker for the id
	// and reports an ErrNotFound error if the worker
	// isn't found.
	getWorker := func() (Worker, error) {
		info := runner.workers[id]
		if info == nil {
			// No entry for the id means the worker
			// will never become available.
			return nil, ErrNotFound
		}
		return info.worker, nil
	}
	if w, err := getWorker(); err != nil || w != nil {
		// The worker is immediately available  (or we know it's
		// not going to become available). No need
		// to block waiting for it.
		runner.mu.Unlock()
		return w, err
	}
	type workerResult struct {
		w   Worker
		err error
	}
	wc := make(chan workerResult, 1)
	stopped := false
	go func() {
		defer runner.mu.Unlock()
		for !stopped {
			// Note: sync.Condition.Wait unlocks the mutex before
			// waiting, then locks it again before returning.
			runner.workersChangedCond.Wait()
			if w, err := getWorker(); err != nil || w != nil {
				wc <- workerResult{w, err}
				return
			}
		}
	}()
	select {
	case w := <-wc:
		if w.err != nil && errors.Cause(w.err) == ErrNotFound {
			// If it wasn't found, it's possible that's because
			// the whole thing has shut down, so
			// check for dying so that we don't mislead
			// our caller.
			select {
			case <-runner.tomb.Dying():
				return nil, ErrDead
			default:
			}
		}
		return w.w, w.err
	case <-runner.tomb.Dying():
		return nil, ErrDead
	case <-stop:
	}
	// Stop our wait goroutine.
	// Strictly speaking this can wake up more waiting Worker calls
	// than needed, but this shouldn't be a problem as in practice
	// almost all the time Worker should not need to start the
	// goroutine.
	runner.mu.Lock()
	stopped = true
	runner.mu.Unlock()
	runner.workersChangedCond.Broadcast()
	return nil, ErrStopped
}

func (runner *Runner) run() error {
	tombDying := runner.tomb.Dying()
	for {
		if runner.isDying && len(runner.workers) == 0 {
			return runner.finalError
		}
		select {
		case <-tombDying:
			logger.Infof("runner is dying")
			runner.isDying = true
			runner.killAll()
			tombDying = nil

		case req := <-runner.startc:
			logger.Debugf("start %q", req.id)
			runner.startWorker(req)
			close(req.reply)

		case id := <-runner.stopc:
			logger.Debugf("stop %q", id)
			runner.killWorker(id)

		case info := <-runner.startedc:
			logger.Debugf("%q started", info.id)
			runner.setWorker(info.id, info.worker)

		case info := <-runner.donec:
			logger.Debugf("%q done: %v", info.id, info.err)
			runner.workerDone(info)
		}
		runner.workersChangedCond.Broadcast()
	}
}

// startWorker responds when a worker has been started
// by calling StartWorker.
func (runner *Runner) startWorker(req startReq) {
	if runner.isDying {
		logger.Infof("ignoring start request for %q when dying", req.id)
		return
	}
	info := runner.workers[req.id]
	if info == nil {
		runner.mu.Lock()
		defer runner.mu.Unlock()
		runner.workers[req.id] = &workerInfo{
			start:        req.start,
			restartDelay: runner.params.RestartDelay,
		}
		go runner.runWorker(0, req.id, req.start)
		return
	}
	if !info.stopping {
		// The worker is already running, so leave it alone
		return
	}
	// The worker previously existed and is
	// currently being stopped.  When it eventually
	// does stop, we'll restart it immediately with
	// the new start function.
	info.start = req.start
	info.restartDelay = 0
}

// workerDone responds when a worker has finished or failed
// to start. It maintains the runner.finalError field and
// restarts the worker if necessary.
func (runner *Runner) workerDone(info doneInfo) {
	workerInfo := runner.workers[info.id]
	if !workerInfo.stopping && info.err == nil {
		logger.Debugf("removing %q from known workers", info.id)
		runner.removeWorker(info.id)
		return
	}
	if info.err != nil {
		if runner.params.IsFatal(info.err) {
			logger.Errorf("fatal %q: %v", info.id, info.err)
			if runner.finalError == nil || runner.params.MoreImportant(info.err, runner.finalError) {
				runner.finalError = info.err
			}
			runner.removeWorker(info.id)
			if !runner.isDying {
				runner.isDying = true
				runner.killAll()
			}
			return
		}
		logger.Errorf("exited %q: %v", info.id, info.err)
	}
	if workerInfo.start == nil {
		logger.Debugf("no restart, removing %q from known workers", info.id)

		// The worker has been deliberately stopped;
		// we can now remove it from the list of workers.
		runner.removeWorker(info.id)
		return
	}
	go runner.runWorker(workerInfo.restartDelay, info.id, workerInfo.start)
	workerInfo.restartDelay = runner.params.RestartDelay
}

// removeWorker removes the worker with the given id from the
// set of current workers. This should only be called when
// the worker is not running.
func (runner *Runner) removeWorker(id string) {
	runner.mu.Lock()
	delete(runner.workers, id)
	runner.mu.Unlock()
}

// setWorker sets the worker associated with the given id.
func (runner *Runner) setWorker(id string, w Worker) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	info := runner.workers[id]
	info.worker = w
	if runner.isDying || info.stopping {
		// We're dying or the worker has already been
		// stopped, so kill it already.
		runner.killWorkerLocked(id)
	}
}

// killAll stops all the current workers.
func (runner *Runner) killAll() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	for id := range runner.workers {
		runner.killWorkerLocked(id)
	}
}

// killWorker stops the worker with the given id, and
// marks it so that it will not start again unless explicitly started
// with StartWorker.
func (runner *Runner) killWorker(id string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.killWorkerLocked(id)
}

// killWorkerLocked is like killWorker except that it expects
// the runner.mu mutex to be held already.
func (runner *Runner) killWorkerLocked(id string) {
	info := runner.workers[id]
	if info == nil {
		return
	}
	info.stopping = true
	info.start = nil
	if info.worker != nil {
		logger.Debugf("killing %q", id)
		info.worker.Kill()
		info.worker = nil
	} else {
		logger.Debugf("couldn't kill %q, not yet started", id)
	}
}

// runWorker starts the given worker after waiting for the given delay.
func (runner *Runner) runWorker(delay time.Duration, id string, start func() (Worker, error)) {
	if delay > 0 {
		logger.Infof("restarting %q in %v", id, delay)
		// TODO(rog) provide a way of interrupting this
		// so that it can be stopped when a worker is removed.
		select {
		case <-runner.tomb.Dying():
			runner.donec <- doneInfo{id, nil}
			return
		case <-runner.params.Clock.After(delay):
		}
	}
	logger.Infof("start %q", id)

	// Defensively ensure that we get reasonable behaviour
	// if something calls Goexit inside the worker (this can
	// happen if something calls check.Assert) - if we don't
	// do this, then this will usually turn into a hard-to-find
	// deadlock when the runner is stopped but the runWorker
	// goroutine will never signal that it's finished.
	normal := false
	defer func() {
		if normal {
			return
		}
		// Since normal isn't true, it means that something
		// inside start must have called panic or runtime.Goexit.
		// If it's a panic, we let it panic; if it's called Goexit,
		// we'll just return an error, enabling this functionality
		// to be tested.
		if err := recover(); err != nil {
			panic(err)
		}
		logger.Infof("%q called runtime.Goexit unexpectedly", id)
		runner.donec <- doneInfo{id, errors.Errorf("runtime.Goexit called in running worker - probably inappropriate Assert")}
	}()
	worker, err := start()
	normal = true

	if err == nil {
		runner.startedc <- startInfo{id, worker}
		err = worker.Wait()
	}
	logger.Infof("stopped %q, err: %v", id, err)
	runner.donec <- doneInfo{id, err}
}
