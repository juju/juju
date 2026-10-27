// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package worker provides small, general-purpose workers and helpers for
// running workers.
//
// A worker is any type that implements the worker.Worker interface of
// github.com/juju/worker/v5 (Kill and Wait). This package provides the
// simplest ones, which run a function on their own goroutine:
//
//   - NewSimpleWorker runs a function once. The function receives a
//     context that is cancelled when the worker is killed, and the error it
//     returns is returned by the worker's Wait method.
//   - NewPeriodicWorker calls a function repeatedly, the first time
//     immediately and then after the given period, until the worker is
//     killed. The function receives a context that is cancelled when the
//     worker is killed, and the error it returns is returned by the
//     worker's Wait method.
//   - NoopWorker returns a worker that waits for its context to be done, and
//     FinishedWorker is a worker that stops immediately with no error.
//
// It also provides the Runner interface, implemented by instances that can
// start and stop workers by id, the RestartDelay a runner waits before
// restarting a worker, and the errors (ErrRestartAgent, ErrTerminateAgent,
// ErrRebootMachine, ErrShutdownMachine) that workers return to ask the agent
// that runs them to take an action.
//
// See github.com/juju/juju/core/watcher for workers that react to watcher
// events, and github.com/juju/worker/v5 for the dependency engine and the
// catacomb that Juju workers are built with.
package worker
