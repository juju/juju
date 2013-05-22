package tasks

import (
	"errors"
	"launchpad.net/juju-core/log"
	"launchpad.net/tomb"
	"time"
)

// RestartDelay holds the length of time that a task
// will wait between exiting and restarting.
var RestartDelay = 3 * time.Second

// Task is implemented by a running task.
type Task interface {
	// Kill asks the task to stop without necessarily
	// waiting for it to do so.
	Kill()
	// Wait waits for the task to exit and returns any
	// error encountered when it was running.
	Wait() error
}

// Runner runs a set of tasks, restarting them as necessary
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
	start func() (Task, error)
}

type startInfo struct {
	id   string
	task Task
}

type doneInfo struct {
	id  string
	err error
}

// NewRunner creates a new Runner.  When a task finishes, if its error
// is deemed fatal (determined by calling isFatal), all the other tasks
// will be stopped and the runner itself will finish.  Of all the errors
// returned by the stopped tasks, only the most important one
// (determined by calling moreImportant) will be returned from
// Runner.Wait.
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

var ErrDead = errors.New("runner is not running")

// StartTask starts a task running associated with the given id.
// The startFunc function will be called to create the task;
// when the task exits, it will be restarted as long as it
// does not return a fatal error.
//
// If there is already a task with the given id, nothing will be done.
//
// StartTask returns ErrDead if the runner is not running.
func (runner *Runner) StartTask(id string, startFunc func() (Task, error)) error {
	select {
	case runner.startc <- startReq{id, startFunc}:
		return nil
	case <-runner.tomb.Dead():
		return ErrDead
	}
}

// StopTask stops the task associated with the given id.
// It does nothing if there is no such task.
//
// StartTask returns ErrDead if the runner is not running.
func (runner *Runner) StopTask(id string) error {
	select {
	case runner.stopc <- id:
		return nil
	case <-runner.tomb.Dead():
		return ErrDead
	}
}

func (runner *Runner) Wait() error {
	return runner.tomb.Wait()
}

func (runner *Runner) Kill() {
	runner.tomb.Kill(nil)
}

// Stop kills the runner and waits for it to exit.
func (runner *Runner) Stop() error {
	runner.Kill()
	return runner.Wait()
}

func (runner *Runner) run() error {
	type taskInfo struct {
		start        func() (Task, error)
		task         Task
		restartDelay time.Duration
	}
	// tasks holds the current set of tasks.  All tasks with a
	// running goroutine have an entry here, even if they are being
	// stopped.  Entries that have been stopped will have a nil
	// start field.
	tasks := make(map[string]*taskInfo)
	var finalError error
loop:
	for {
		select {
		case <-runner.tomb.Dying():
			break loop
		case req := <-runner.startc:
			info := tasks[req.id]
			if info != nil && info.start != nil {
				// The task is already around; no need to do anything.
				break
			}
			if info != nil {
				// The task previously existed and is
				// currently being stopped.  When it
				// eventually does stop, we'll restart
				// it immediately with the new start
				// function.
				info.start = req.start
				info.restartDelay = 0
			} else {
				tasks[req.id] = &taskInfo{
					start: req.start,
					restartDelay: RestartDelay,
				}
				go runner.runTask(0, req.id, req.start)
			}
		case id := <-runner.stopc:
			info := tasks[id]
			if info == nil || info.start == nil {
				// The task doesn't exist or is already being stopped.
				break
			}
			if info.task != nil {
				info.task.Kill()
				info.task = nil
			}
			info.start = nil
		case info := <-runner.startedc:
			tasks[info.id].task = info.task
		case info := <-runner.donec:
			taskInfo := tasks[info.id]
			if taskInfo.start != nil && info.err == nil {
				info.err = errors.New("unexpected quit")
			}
			if info.err != nil {
				log.Errorf("tasks: task %q: %v", info.id, info.err)
				if runner.isFatal(info.err) {
					finalError = info.err
					break loop
				}
			}
			if taskInfo.start == nil {
				// The task has been deliberately stopped;
				// we can now remove it from the list of tasks.
				delete(tasks, info.id)
				break
			}
			go runner.runTask(taskInfo.restartDelay, info.id, taskInfo.start)
			taskInfo.restartDelay = RestartDelay
		}
	}
	for _, info := range tasks {
		if info.task != nil {
			info.task.Kill()
			info.task = nil
		}
	}
	for len(tasks) > 0 {
		info := <-runner.donec
		if runner.moreImportant(info.err, finalError) {
			finalError = info.err
		}
		if info.err != nil {
			log.Errorf("task %q: %v", info.id, info.err)
		}
		delete(tasks, info.id)
	}
	return finalError
}

// runTask starts the given task after waiting for the given delay.
func (runner *Runner) runTask(delay time.Duration, id string, start func() (Task, error)) {
	if delay > 0 {
		select {
		case <-runner.tomb.Dying():
			runner.donec <- doneInfo{id, nil}
			return
		case <-time.After(delay):
		}
	}
	task, err := start()
	if err == nil {
		runner.startedc <- startInfo{id, task}
		err = task.Wait()
	}
	runner.donec <- doneInfo{id, err}
}
